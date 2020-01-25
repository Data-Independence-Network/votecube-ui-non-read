package main

import (
	"bitbucket.org/votecube/votecube-go-lib/model/scylladb"
	"bitbucket.org/votecube/votecube-go-lib/sequence"
	"bitbucket.org/votecube/votecube-go-lib/utils"
	"database/sql"
	"flag"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/fasthttp/router"
	"github.com/gocql/gocql"
	_ "github.com/lib/pq"
	"github.com/robfig/cron"
	"github.com/scylladb/gocqlx"
	"github.com/scylladb/gocqlx/qb"
	"github.com/valyala/fasthttp"
)

var (
	DB                         *sql.DB
	scdbHosts                  = flag.String("scdbHosts", "localhost", "TCP address to listen to")
	crdbPath                   = flag.String("crdbPath", "root@localhost:26257", "TCP address to listen to")
	addr                       = flag.String("addr", ":8445", "TCP address to listen to")
	cluster                    *gocql.ClusterConfig
	session                    *gocql.Session
	err                        error
	partitionPeriod            = utils.GetCurrentDateMinute()
	insertOpinion              *gocqlx.Queryx
	insertPoll                 *gocqlx.Queryx
	insertOpinionUpdate        *gocqlx.Queryx
	selectPollId               *gocqlx.Queryx
	selectPollAndRootOpinionId *gocqlx.Queryx
	selectParentOpinionData    *gocqlx.Queryx
	selectPreviousOpinionData  *gocqlx.Queryx
	updateOpinion              *gocqlx.Queryx
	opinionInsertBatchId       int32 = 0
	opinionUpdateBatchId       int32 = 0
	pollInsertBatchId          int32 = 0
	//compress = flag.Bool("compress", false, "Whether to enable transparent response compression")
)

type OpinionData struct {
	CreateEs        int64  `json:"createEs"`
	Id              int64  `json:"id"`
	ParentOpinionId int64  `json:parentId,omitempty`
	PollId          int64  `json:"pollId"`
	Text            string `json:"text"`
	UserId          int64  `json:"userId"`
}

type PollData struct {
	Contents string `json:"contents"` // `json:"contents,omitempty"`
	CreateEs int64  `json:"createEs"`
	Id       int64  `json:"id"`
	Title    string `json:"title"`
	UserId   int64  `json:"userId"`
}

func AddOpinion(ctx *fasthttp.RequestCtx) {
	ctx.SetUserValue("recordType", "opinion")

	requestBytes := (*ctx).Request.Body()
	opinionData := OpinionData{}
	if !utils.Unmarshal(requestBytes, &opinionData, ctx) {
		return
	}

	var (
		okPollOrParentOpinionExists bool
		parentOpinionRows           []scylladb.Opinion
		parentPollRows              []scylladb.Poll
		rootOpinionId               int64
		version                     int16
		waitGroup                   sync.WaitGroup
	)
	userContext := utils.NewParallelUserContext(ctx, opinionData.UserId, waitGroup)
	if userContext == nil {
		return
	}

	// If there is a parent opinion query for it
	// Otherwise query the poll to ensure it exists

	waitGroup.Add(2)
	// TODO: test all (2 or more) queries are failing at the same time
	go utils.GetUserSession(*userContext)
	go func() {
		defer waitGroup.Done()

		if opinionData.ParentOpinionId == 0 {
			selectPollIdQuery := selectPollId.BindMap(qb.M{
				"poll_id": opinionData.PollId,
			})
			okPollOrParentOpinionExists =
				utils.Select(selectPollIdQuery, &parentPollRows, ctx)
		} else {
			selectParentOpinionDataQuery := selectParentOpinionData.BindMap(qb.M{
				"opinion_id": opinionData.ParentOpinionId,
			})
			okPollOrParentOpinionExists =
				utils.Select(selectParentOpinionDataQuery, &parentOpinionRows, ctx)
		}
	}()
	waitGroup.Wait()

	if !okPollOrParentOpinionExists || !utils.CheckSession(*userContext) {
		return
	}

	rootOpinionId = 0
	if opinionData.ParentOpinionId == 0 {
		if len(parentPollRows) != 1 {
			log.Printf("Did not find poll for poll_id: %d", opinionData.PollId)
			ctx.Error("Internal Server Error", http.StatusInternalServerError)
			return
		}
	} else {
		if len(parentOpinionRows) != 1 {
			log.Printf("Did not find parent opinion opinion_id: %d", opinionData.ParentOpinionId)
			ctx.Error("Internal Server Error", http.StatusInternalServerError)
			return
		}
		parentOpinion := parentOpinionRows[0]
		opinionData.PollId = parentOpinion.PollId
		rootOpinionId = parentOpinion.RootOpinionId
	}

	opinionId, ok := utils.GetSeq(sequence.OpinionId, ctx)
	if !ok {
		return
	}

	opinionData.Id = opinionId
	createEs := utils.GetCurrentEs()
	opinionData.CreateEs = createEs

	compressedOpinion, ok := utils.MarshalZip(opinionData, ctx)
	if !ok {
		return
	}

	version = 1
	opinion := scylladb.Opinion{
		PollId:          opinionData.PollId,
		PartitionPeriod: partitionPeriod,
		AgeSuitability:  0,
		OpinionId:       opinionId,
		ThemeId:         0,
		LocationId:      0,
		IngestBatchId:   opinionInsertBatchId,
		Version:         version,
		RootOpinionId:   rootOpinionId,
		ParentOpinionId: opinionData.ParentOpinionId,
		CreateEs:        createEs,
		UserId:          userContext.UserId,
		Data:            compressedOpinion.Bytes(),
		InsertProcessed: false,
	}

	opinionInsertBatchId = (opinionInsertBatchId + 1) % 16

	if !utils.Insert(insertOpinion, opinion, ctx) {
		return
	}
	utils.ReturnId(opinionId, ctx)
}

func UpdateOpinion(ctx *fasthttp.RequestCtx) {
	ctx.SetUserValue("recordType", "opinionUpdate")

	requestBytes := (*ctx).Request.Body()
	opinionData := OpinionData{}
	if !utils.Unmarshal(requestBytes, &opinionData, ctx) {
		return
	}

	// TODO: verify structure of the data

	var (
		okPreviousPosition bool
		previousOpinions   []scylladb.Opinion
		previousOpinion    scylladb.Opinion
		version            int16
		waitGroup          sync.WaitGroup
	)
	userContext := utils.NewParallelUserContext(ctx, opinionData.UserId, waitGroup)
	if userContext == nil {
		return
	}

	waitGroup.Add(2)
	go utils.GetUserSession(*userContext)
	go func() {
		defer waitGroup.Done()

		selectParentPositionQuery := selectPreviousOpinionData.BindMap(qb.M{
			"opinion_id": opinionData.Id,
		})

		okPreviousPosition = utils.Select(selectParentPositionQuery, &previousOpinions, ctx)
	}()
	waitGroup.Wait()

	if !okPreviousPosition || !utils.CheckSession(*userContext) {
		return
	}

	if len(previousOpinions) != 1 {
		log.Printf("Did not find an opinion record with opinion_id: %d", opinionData.Id)
		ctx.Error("Internal Server Error", http.StatusInternalServerError)
		return
	}

	previousOpinion = previousOpinions[0]

	// TODO: Move this check to Auth rules a la Firebase rules
	if previousOpinion.UserId != (*userContext).UserId {
		log.Printf("Opinion user_id: %d does not match provided user_id: %d",
			previousOpinion.UserId, (*userContext).UserId)
		ctx.Error("Internal Server Error", http.StatusInternalServerError)
		return
	}

	// TODO: this can also be done with Firebase style rules
	opinionData.PollId = previousOpinion.PollId
	opinionData.ParentOpinionId = previousOpinion.ParentOpinionId
	opinionData.CreateEs = previousOpinion.CreateEs

	/**
	Version never should but technically can overflow (to negative).  This is OK since version
	isn't part of the ID is needed only for caching Opinion responses in CDN and browser.
	*/
	version = previousOpinion.Version + 1 // no need to % math.Exp2(15), this runs OK on
	// overflow since previousOpinion.Version is int16 (it now goes to -2^15)
	/*
			Tested on https://play.golang.org/ with:
		func main() {
			var test int16 = int16(math.Exp2(15) - 1)
			fmt.Printf("Before overflow: %d\n", test)
			test = test + 1
			fmt.Printf("After overflow:  %d\n", test)
		}
	*/

	compressedOpinion, ok := utils.MarshalZip(opinionData, ctx)
	if !ok {
		return
	}

	opinionSetClause := scylladb.Opinion{
		Version: version,
		Data:    compressedOpinion.Bytes(),
	}
	updateOpinionQuery := updateOpinion.BindMap(qb.M{
		"opinion_id": opinionData.Id,
	})
	if !utils.Update(updateOpinionQuery, opinionSetClause, ctx) {
		return
	}

	updatePeriod := utils.GetDateMinuteFromEpochSeconds(opinionData.CreateEs)
	if updatePeriod == partitionPeriod {
		// No, need to create an update record, the original record hasn't yet
		// been picked up by the batch process
		utils.ReturnShortVersion(version, ctx)
		return
	}

	opinionUpdateBatchId = (opinionUpdateBatchId + 1) % 16

	opinionUpdate := scylladb.OpinionUpdate{
		PollId:          opinionData.PollId,
		PartitionPeriod: partitionPeriod,
		IngestBatchId:   opinionUpdateBatchId,
		OpinionId:       opinionData.Id,
		Version:         version,
		UpdateProcessed: false,
	}

	if !utils.Insert(insertOpinionUpdate, opinionUpdate, ctx) {
		return
	}

	utils.ReturnShortVersion(version, ctx)
}

func AddPoll(ctx *fasthttp.RequestCtx) {
	ctx.SetUserValue("recordType", "poll")

	requestBytes := (*ctx).Request.Body()
	pollData := PollData{}
	if !utils.Unmarshal(requestBytes, &pollData, ctx) {
		return
	}
	if !utils.IsValidSession(ctx, pollData.UserId) {
		return
	}

	pollId, ok := utils.GetSeq(sequence.PollId, ctx)
	if !ok {
		return
	}

	pollData.Id = pollId
	createEs := utils.GetCurrentEs()
	pollData.CreateEs = createEs

	compressedPoll, ok := utils.MarshalZip(pollData, ctx)
	if !ok {
		return
	}

	poll := scylladb.Poll{
		PollId:          pollId,
		ThemeId:         1,
		LocationId:      1,
		IngestBatchId:   pollInsertBatchId,
		UserId:          pollData.UserId,
		CreateEs:        createEs,
		PartitionPeriod: partitionPeriod,
		AgeSuitability:  0,
		Data:            compressedPoll.Bytes(),
		InsertProcessed: false,
	}

	pollInsertBatchId = (pollInsertBatchId + 1) % 16

	if !utils.Insert(insertPoll, poll, ctx) {
		return
	}
	utils.ReturnId(pollId, ctx)
}

func main() {
	DB = utils.SetupDb(*crdbPath)
	sequence.SetupSequences(DB)
	defer DB.Close()

	flag.Parse()
	c := cron.New(cron.WithLocation(time.UTC))
	//c.AddFunc("0,15,30,45 * * * *", everyPartitionPeriod)
	//c.AddFunc("0,10,20,30,40,50 * * * *", everyPartitionPeriod)
	c.AddFunc("0,5,10,15,20,25,30,35,40,45,50,55 * * * *", everyPartitionPeriod)
	c.Start()

	// connect to the ScyllaDB cluster
	cluster = gocql.NewCluster(strings.SplitN(*scdbHosts, ",", -1)...)

	cluster.PoolConfig.HostSelectionPolicy = gocql.TokenAwareHostPolicy(gocql.RoundRobinHostPolicy())
	//cluster.Compressor = &gocql.SnappyCompressor{}
	cluster.RetryPolicy = &gocql.ExponentialBackoffRetryPolicy{NumRetries: 3}
	cluster.Consistency = gocql.Any

	cluster.Keyspace = "votecube"

	session, err = cluster.CreateSession()

	if err != nil {
		// unable to connect
		panic(err)
	}
	defer session.Close()

	utils.SetupAuthQueries(session)

	stmt, names := qb.Insert("opinions").Columns(
		"opinion_id",
		"poll_id",
		"create_period",
		"user_id",
		"create_es",
		"update_es",
		"version",
		"data",
		"insert_processed",
	).ToCql()
	insertOpinion = gocqlx.Query(session.Query(stmt), names)

	stmt, names = qb.Update("opinions").Set(
		"update_es",
		"version",
		"data",
	).Where(
		qb.Eq("poll_id"),
		qb.Eq("create_period"),
		qb.Eq("create_es"),
		qb.Eq("opinion_id"),
	).ToCql()
	updateOpinion = gocqlx.Query(session.Query(stmt), names)

	stmt, names = qb.Insert("opinion_updates").Columns(
		"opinion_id",
		"poll_id",
		"update_period",
		"user_id",
		"update_es",
		"data",
		"version",
		"update_processed",
	).ToCql()
	insertOpinionUpdate = gocqlx.Query(session.Query(stmt), names)

	stmt, names = qb.Insert("polls").Columns(
		"poll_id",
		"theme_id",
		"location_id",
		"user_id",
		"date",
		"create_es",
		"data",
		"batch_id",
	).ToCql()
	insertPoll = gocqlx.Query(session.Query(stmt), names)

	stmt, names = qb.Select("opinions").Columns(
		"root_opinion_id",
	).Where(
		qb.Eq("opinion_id"),
	).BypassCache().ToCql()
	selectParentOpinionData = gocqlx.Query(session.Query(stmt), names)

	stmt, names = qb.Select("opinions").Columns(
		"user_id",
		"version",
		"parent_id",
		"poll_id",
		"create_es",
	).Where(
		qb.Eq("poll_id"),
		qb.Eq("create_period"),
		qb.Eq("create_es"),
		qb.Eq("opinion_id"),
	).BypassCache().ToCql()
	selectPreviousOpinionData = gocqlx.Query(session.Query(stmt), names)

	stmt, names = qb.Select("polls").Columns(
		"poll_id",
	).Where(
		qb.Eq("poll_id"),
	).BypassCache().ToCql()
	selectPollId = gocqlx.Query(session.Query(stmt), names)

	stmt, names = qb.Select("opinions").Columns(
		"poll_id",
		"root_opinion_id",
	).Where(
		qb.Eq("poll_id"),
	).BypassCache().ToCql()
	selectPollAndRootOpinionId = gocqlx.Query(session.Query(stmt), names)

	r := router.New()
	r.PUT("/put/opinion/:sessionPartitionPeriod/:sessionId", AddOpinion)
	r.PUT("/update/opinion/:sessionPartitionPeriod/:sessionId", UpdateOpinion)
	r.PUT("/put/poll/:sessionPartitionPeriod/:sessionId", AddPoll)

	log.Fatal(fasthttp.ListenAndServe(*addr, r.Handler))
}

func everyPartitionPeriod() {
	partitionPeriod = utils.GetCurrentDateMinute()
}

/**
wg                  sync.WaitGroup

	wg.Add(numQueries)
*/
