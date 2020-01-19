package main

import (
	"bitbucket.org/votecube/votecube-go-lib/model/scylladb"
	"bitbucket.org/votecube/votecube-go-lib/sequence"
	"bitbucket.org/votecube/votecube-go-lib/utils"
	"database/sql"
	"flag"
	"log"
	"net/http"
	"strconv"
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
	DB                        *sql.DB
	scdbHosts                 = flag.String("scdbHosts", "localhost", "TCP address to listen to")
	crdbPath                  = flag.String("crdbPath", "root@localhost:26257", "TCP address to listen to")
	addr                      = flag.String("addr", ":8445", "TCP address to listen to")
	cluster                   *gocql.ClusterConfig
	session                   *gocql.Session
	err                       error
	dateHour                  = utils.GetDateHour()
	insertOpinion             *gocqlx.Queryx
	insertPoll                *gocqlx.Queryx
	insertThread              *gocqlx.Queryx
	insertOpinionUpdate       *gocqlx.Queryx
	selectNumChildOpinions    *gocqlx.Queryx
	selectPollId              *gocqlx.Queryx
	selectParentOpinionData   *gocqlx.Queryx
	selectPreviousOpinionData *gocqlx.Queryx
	updateOpinion             *gocqlx.Queryx
	batchId                   = -1
	//compress = flag.Bool("compress", false, "Whether to enable transparent response compression")
)

type OpinionData struct {
	Id       uint64 `json:"id"`
	PollId   uint64 `json:"pollId"`
	ParentId uint64 `json:parentId,omitempty`
	CreateEs int64  `json:"createEs"`
	UpdateEs int64  `json:"createEs,omitempty"`
	Text     string `json:"text"`
}

type PollData struct {
	Id       uint64 `json:"id"`
	CreateEs int64  `json:"createEs"`
	Title    string `json:"title"`
	Contents string `json:"contents"` // `json:"contents,omitempty"`
}

func AddOpinion(ctx *fasthttp.RequestCtx) {
	ctx.SetUserValue("recordType", "opinion")

	parentCreateEs, ok := utils.ParseInt64Param("parentCreateEs", ctx)
	if !ok {
		return
	}
	requestBytes := (*ctx).Request.Body()
	opinionData := OpinionData{}
	if !utils.Unmarshal(requestBytes, &opinionData, ctx) {
		return
	}

	var (
		numChildOpinions    int
		okNumChildPositions bool
		okParentPosition    = true
		okPollExists        bool
		parentOpinions      []scylladb.Opinion
		pollExistsRows      []scylladb.Poll
		position            string
		rootOpinionId       uint64
		version             uint16
		wg                  sync.WaitGroup
	)
	selectPollIdQuery := selectPollId.BindMap(qb.M{
		"poll_id": opinionData.PollId,
	})
	selectNumChildOpinionsQuery := selectNumChildOpinions.BindMap(qb.M{
		"poll_id":   opinionData.PollId,
		"parent_id": opinionData.ParentId,
	})

	numQueries := 2
	if opinionData.ParentId != 0 {
		numQueries = 3
	}

	wg.Add(numQueries)
	// TODO: test all (2 or more) queries are failing at the same time
	go func() {
		defer wg.Done()
		okPollExists =
			utils.Select(selectPollIdQuery, &pollExistsRows, ctx)
	}()
	go func() {
		defer wg.Done()
		okNumChildPositions =
			utils.SelectCount(selectNumChildOpinionsQuery, &numChildOpinions, ctx)
	}()

	if opinionData.ParentId != 0 {
		go func() {
			defer wg.Done()

			selectParentPositionQuery := selectParentOpinionData.BindMap(qb.M{
				"poll_id":     opinionData.PollId,
				"create_hour": utils.GetDateHourFromEpochSeconds(parentCreateEs),
				"create_es":   parentCreateEs,
				"opinion_id":  opinionData.ParentId,
			})

			okParentPosition =
				utils.Select(selectParentPositionQuery, &parentOpinions, ctx)
		}()
	}
	wg.Wait()

	if !okPollExists || !okNumChildPositions || !okParentPosition {
		return
	}

	if len(pollExistsRows) != 1 {
		log.Printf("Did not find a poll record with poll_id: %d", opinionData.PollId)
		ctx.Error("Internal Server Error", http.StatusInternalServerError)
		return
	}

	position = strconv.Itoa(numChildOpinions)

	rootOpinionId = 0
	if opinionData.ParentId != 0 {
		if len(parentOpinions) != 1 {
			log.Printf("Did not find parent opinion record for poll_id: %d, create_es: %d, opinion_id: %d", opinionData.PollId, parentCreateEs, opinionData.ParentId)
			ctx.Error("Internal Server Error", http.StatusInternalServerError)
			return
		}
		parentOpinion := parentOpinions[0]
		position = parentOpinion.Position + "." + position
		if parentOpinion.RootOpinionId == 0 {
			rootOpinionId = opinionData.ParentId
		} else {
			rootOpinionId = parentOpinion.RootOpinionId
		}
	}

	opinionId, ok := utils.GetSeq(sequence.OpinionId, ctx)
	if !ok {
		return
	}

	opinionData.Id = opinionId
	createEs := time.Now().Unix()
	opinionData.CreateEs = createEs

	compressedOpinion, ok := utils.MarshalZip(opinionData, ctx)
	if !ok {
		return
	}

	version = 1
	opinion := scylladb.Opinion{
		OpinionId:       opinionId,
		RootOpinionId:   rootOpinionId,
		PollId:          opinionData.PollId,
		Position:        position,
		CreateHour:      dateHour,
		UserId:          1,
		CreateEs:        createEs,
		UpdateEs:        0,
		Version:         version,
		Data:            compressedOpinion.Bytes(),
		InsertProcessed: false,
	}

	if !utils.Insert(insertOpinion, opinion, ctx) {
		return
	}
	utils.ReturnIdAndCreateEsAndVersion(opinionId, createEs, version, ctx)
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
		updateData         []byte
		updateProcessed    bool
		version            uint16
	)

	createHour := utils.GetDateHourFromEpochSeconds(opinionData.CreateEs)

	selectParentPositionQuery := selectPreviousOpinionData.BindMap(qb.M{
		"poll_id":     opinionData.PollId,
		"create_hour": createHour,
		"create_es":   opinionData.CreateEs,
		"opinion_id":  opinionData.Id,
	})

	okPreviousPosition = utils.Select(selectParentPositionQuery, &previousOpinions, ctx)
	if !okPreviousPosition {
		return
	}

	if len(previousOpinions) != 1 {
		log.Printf("Did not find a poll record with poll_id: %d", opinionData.PollId)
		ctx.Error("Internal Server Error", http.StatusInternalServerError)
		return
	}

	previousOpinion = previousOpinions[0]

	// TODO: verify that the opinion belongs to the specified user

	/**
	Version never should but technically can overflow (to 0).  This is OK since version
	isn't part of the ID is needed only for caching Opinion responses in CDN and browser.
	Since the cache only lasts a day and it never SHOULD overflow (especially in one day)
	it is OK.
	*/
	version = previousOpinion.Version + 1 // no need to % 65536, this runs OK on overflow
	// since previousOpinion.Version is uint16
	/*
		Tested on https://play.golang.org/ with:
		func main() {
			var (
				val uint16
				val2 uint16
			)
			val = 65535
			val2 = val + 1
			fmt.Printf("Overflow Assign %d\n", val2) // Overflow Assign 0
		}
	*/

	updateEs := time.Now().Unix()

	opinionData.ParentId = previousOpinion.ParentId // Should not be in input
	opinionData.UpdateEs = updateEs

	compressedOpinion, ok := utils.MarshalZip(opinionData, ctx)
	if !ok {
		return
	}

	version = 1
	opinionSetClause := scylladb.Opinion{
		UpdateEs: updateEs,
		Version:  version,
		Data:     compressedOpinion.Bytes(),
	}

	updateOpinionQuery := updateOpinion.BindMap(qb.M{
		"poll_id":     opinionData.PollId,
		"create_hour": createHour,
		"create_es":   opinionData.CreateEs,
		"opinion_id":  opinionData.Id,
	})

	if !utils.Update(updateOpinionQuery, opinionSetClause, ctx) {
		return
	}

	updateHour := utils.GetDateHourFromEpochSeconds(updateEs)
	if updateHour == createHour {
		updateData = nil
		updateProcessed = true
	} else {
		updateData = compressedOpinion.Bytes()
		updateProcessed = false
	}

	opinionUpdate := scylladb.OpinionUpdate{
		OpinionId:       opinionData.Id,
		PollId:          opinionData.PollId,
		UserId:          1,
		UpdateHour:      updateHour,
		UpdateEs:        updateEs,
		Data:            updateData,
		Version:         version,
		UpdateProcessed: updateProcessed,
	}

	if !utils.Insert(insertOpinionUpdate, opinionUpdate, ctx) {
		return
	}

	utils.ReturnIdAndCreateEsAndVersion(opinionData.Id, opinionData.CreateEs, version, ctx)
}

func AddPoll(ctx *fasthttp.RequestCtx) {
	ctx.SetUserValue("recordType", "poll")

	requestBytes := (*ctx).Request.Body()
	pollData := PollData{}
	if !utils.Unmarshal(requestBytes, &pollData, ctx) {
		return
	}
	pollId, ok := utils.GetSeq(sequence.PollId, ctx)
	if !ok {
		return
	}

	pollData.Id = pollId
	createEs := time.Now().Unix()
	pollData.CreateEs = createEs

	compressedPoll, ok := utils.MarshalZip(pollData, ctx)
	if !ok {
		return
	}

	batchId = (batchId + 1) % 128

	poll := scylladb.Poll{
		PollId:     pollId,
		ThemeId:    1,
		LocationId: 1,
		UserId:     1,
		CreateHour: dateHour,
		CreateEs:   createEs,
		Data:       compressedPoll.Bytes(),
		BatchId:    batchId,
	}
	thread := scylladb.Thread{
		PollId:   pollId,
		UserId:   1,
		CreateEs: createEs,
		Data:     nil,
	}

	if !utils.Insert(insertPoll, poll, ctx) {
		return
	}
	if !utils.Insert(insertThread, thread, ctx) {
		return
	}
	utils.ReturnIdAndCreateEs(pollId, createEs, ctx)
}

func main() {
	DB = utils.SetupDb(*crdbPath)
	sequence.SetupSequences(DB)
	defer DB.Close()

	flag.Parse()
	cron.New(
		cron.WithLocation(time.UTC)).AddFunc("0 * * * *", hourly)
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

	stmt, names := qb.Insert("opinions").Columns(
		"opinion_id",
		"poll_id",
		"create_hour",
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
		qb.Eq("create_hour"),
		qb.Eq("create_es"),
		qb.Eq("opinion_id"),
	).ToCql()
	updateOpinion = gocqlx.Query(session.Query(stmt), names)

	stmt, names = qb.Insert("opinion_updates").Columns(
		"opinion_id",
		"poll_id",
		"update_hour",
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

	stmt, names = qb.Insert("threads").Columns(
		"poll_id",
		"user_id",
		"create_es",
		"data",
	).ToCql()
	insertThread = gocqlx.Query(session.Query(stmt), names)

	stmt, names = qb.Select("opinion_child_count_base").Count(
		"parent_id",
	).Where(
		qb.Eq("poll_id"),
		qb.Eq("parent_id"),
	).BypassCache().ToCql()
	selectNumChildOpinions = gocqlx.Query(session.Query(stmt), names)

	stmt, names = qb.Select("opinions").Columns(
		"position",
		"root_opinion_id",
	).Where(
		qb.Eq("poll_id"),
		qb.Eq("create_hour"),
		qb.Eq("create_es"),
		qb.Eq("opinion_id"),
	).BypassCache().ToCql()
	selectParentOpinionData = gocqlx.Query(session.Query(stmt), names)

	stmt, names = qb.Select("opinions").Columns(
		"user_id",
		"version",
		"parent_id",
	).Where(
		qb.Eq("poll_id"),
		qb.Eq("create_hour"),
		qb.Eq("create_es"),
		qb.Eq("opinion_id"),
	).BypassCache().ToCql()
	selectPreviousOpinionData = gocqlx.Query(session.Query(stmt), names)

	stmt, names = qb.Select("poll_keys").Columns(
		"poll_id",
	).Where(
		qb.Eq("poll_id"),
	).BypassCache().ToCql()
	selectPollId = gocqlx.Query(session.Query(stmt), names)

	r := router.New()
	r.PUT("/put/opinion/:pollId/:parentCreateEs", AddOpinion)
	r.PUT("/update/opinion/:pollId/:createEs/:opinionId", UpdateOpinion)
	r.PUT("/put/poll", AddPoll)

	log.Fatal(fasthttp.ListenAndServe(*addr, r.Handler))
}

func hourly() {
	dateHour = utils.GetDateHour()
}
