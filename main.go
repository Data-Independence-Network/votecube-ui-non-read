package main

import (
	"bitbucket.org/votecube/votecube-go-lib/model/data"
	"bitbucket.org/votecube/votecube-go-lib/model/scylladb"
	"bitbucket.org/votecube/votecube-go-lib/model/vespa"
	"bitbucket.org/votecube/votecube-go-lib/sequence"
	"bitbucket.org/votecube/votecube-go-lib/utils"
	"bitbucket.org/votecube/votecube-go-lib/utils/crdb"
	vespa2 "bitbucket.org/votecube/votecube-go-lib/utils/vespa"
	"bitbucket.org/votecube/votecube-ui-non-read/model"
	"database/sql"
	"flag"
	"github.com/volatiletech/sqlboiler/boil"
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
	DB                                *sql.DB
	scdbHosts                         = flag.String("scdbHosts", "localhost", "TCP address to listen to")
	crdbPath                          = flag.String("crdbPath", "root@localhost:26257", "TCP address to listen to")
	addr                              = flag.String("addr", ":8445", "TCP address to listen to")
	cluster                           *gocql.ClusterConfig
	session                           *gocql.Session
	err                               error
	partitionPeriod                   = utils.GetCurrentPartitionPeriod(5)
	insertOpinion                     *gocqlx.Queryx
	insertOpinionUpdate               *gocqlx.Queryx
	insertPoll                        *gocqlx.Queryx
	insertRootOpinion                 *gocqlx.Queryx
	selectPollId                      *gocqlx.Queryx
	selectParentOpinionData           *gocqlx.Queryx
	selectPreviousOpinionData         *gocqlx.Queryx
	updateOpinion                     *gocqlx.Queryx
	updatePeriodAddedToRootOpinionIds *gocqlx.Queryx
	updatePeriodUpdatedRootOpinionIds *gocqlx.Queryx
	pollIdModFactor                   int64 = 2
	rootOpinionIdModFactor            int64 = 2
	//compress = flag.Bool("compress", false, "Whether to enable transparent response compression")
)

func AddFeedback(ctx *fasthttp.RequestCtx) {
	ctx.SetUserValue("recordType", "feedback")

	requestBytes := (*ctx).Request.Body()
	feedbackData := data.Feedback{}
	if !utils.Unmarshal(requestBytes, &feedbackData, ctx) {
		return
	}

	if feedbackData.AgeSuitability < 0 || feedbackData.AgeSuitability > 10 {
		log.Print("Invalid feedbackTypeId")
		ctx.Error("Internal Server Error", http.StatusInternalServerError)
		return
	}

	if feedbackData.FeedbackTypeId < 0 || feedbackData.FeedbackTypeId > 10 {
		log.Print("Invalid feedbackTypeId")
		ctx.Error("Internal Server Error", http.StatusInternalServerError)
		return
	}

	if feedbackData.UserAccountId != 0 && !utils.IsValidSession(ctx, feedbackData.UserAccountId) {
		return
	}

	feedbackId, ok := utils.GetSeq(&sequence.FeedbackId, ctx)
	if !ok {
		return
	}

	createTime, _, createDtb := utils.GetCurrentDtb()

	feedbackData.Id = feedbackId
	feedbackData.CreateDtb = createDtb

	if !vespa2.AddFeedback("http://localhost:8086", feedbackData, ctx) {
		return
	}

	if !crdb.AddFeedback(feedbackData, createTime, ctx) {
		return
	}

	utils.ReturnId(feedbackId, ctx)
}

func AddFeedbackComment(ctx *fasthttp.RequestCtx) {
	ctx.SetUserValue("recordType", "opinion")

	requestBytes := (*ctx).Request.Body()
	feedbackComment := data.FeedbackComment{}
	if !utils.Unmarshal(requestBytes, &feedbackComment, ctx) {
		return
	}

	var (
		okParentFeedbackExists bool
		feedbackVespa          *vespa.Feedback
		ok                     bool
		waitGroup              sync.WaitGroup
	)
	userContext := utils.NewParallelUserContext(ctx, feedbackComment.UserAccountId, &waitGroup)
	if userContext == nil {
		return
	}

	// If there is a parent opinion query for it
	// Otherwise query the poll to ensure it exists

	waitGroup.Add(2)
	// TODO: test all (2 or more) queries are failing at the same time
	go utils.GetUserSession(userContext)
	go func() {
		defer waitGroup.Done()

		feedbackVespa, ok := vespa2.GetFeedbackById("http://localhost:8086", feedbackComment.FeedbackId, ctx)

		if !ok {
			return
		}

		okParentFeedbackExists = feedbackVespa != nil
	}()
	waitGroup.Wait()

	if !okParentFeedbackExists || !utils.CheckSession(userContext) {
		return
	}

	feedbackCommentId, ok := utils.GetSeq(&sequence.FeedbackCommentId, ctx)
	if !ok {
		return
	}

	createTime, _, createDtb := utils.GetCurrentDtb()
	feedbackComment.CreateDtb = createDtb
	feedbackComment.FeedbackTypeId = feedbackVespa.FeedbackTypeId
	feedbackComment.Id = feedbackCommentId

	if !vespa2.AddFeedbackComment("http://localhost:8086", feedbackComment, ctx) {
		return
	}

	if !crdb.AddFeedbackComment(feedbackComment, createTime, ctx) {
		return
	}

	utils.ReturnId(feedbackCommentId, ctx)
}

func GetLatestFeedbackForType(
	ctx *fasthttp.RequestCtx,
) {

}

func GetLatestFeedbackCommentsForFeedbackId(
	ctx *fasthttp.RequestCtx,
) {

}

func AddOpinion(ctx *fasthttp.RequestCtx) {
	ctx.SetUserValue("recordType", "opinion")

	requestBytes := (*ctx).Request.Body()
	opinionData := data.Opinion{}
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
	userContext := utils.NewParallelUserContext(ctx, opinionData.UserAccountId, &waitGroup)
	if userContext == nil {
		return
	}

	// If there is a parent opinion query for it
	// Otherwise query the poll to ensure it exists

	waitGroup.Add(2)
	// TODO: test all (2 or more) queries are failing at the same time
	go utils.GetUserSession(userContext)
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

	if !okPollOrParentOpinionExists || !utils.CheckSession(userContext) {
		return
	}

	rootOpinionId = 0
	if opinionData.ParentOpinionId == 0 {
		if len(parentPollRows) != 1 {
			log.Printf("Did not find poll for poll_id: %d\n", opinionData.PollId)
			ctx.Error("Internal Server Error", http.StatusInternalServerError)
			return
		}
	} else {
		if len(parentOpinionRows) != 1 {
			log.Printf("Did not find parent opinion opinion_id: %d\n", opinionData.ParentOpinionId)
			ctx.Error("Internal Server Error", http.StatusInternalServerError)
			return
		}

		parentOpinion := data.Opinion{}
		if !utils.UnzipUnmarshal(parentOpinionRows[0].Data, parentOpinion, ctx) {
			return
		}

		opinionData.PollId = parentOpinion.PollId
		rootOpinionId = parentOpinion.RootOpinionId

		rootOpinionIdMod := int32(rootOpinionId % rootOpinionIdModFactor)

		periodAddedToRootIdSetClause := scylladb.PeriodAddedToRootOpinionIds{
			RootOpinionIdMod: rootOpinionIdMod,
		}
		updatePeriodAddedToRootOpinionIdsQuery := updatePeriodAddedToRootOpinionIds.BindMap(qb.M{
			"partition_period":    partitionPeriod,
			"root_opinion_id_mod": rootOpinionIdMod,
		})
		if !utils.Update(
			updatePeriodAddedToRootOpinionIdsQuery, periodAddedToRootIdSetClause, ctx) {
			return
		}
	}

	opinionId, ok := utils.GetSeq(&sequence.OpinionId, ctx)
	if !ok {
		return
	}

	_, _, createDtb := utils.GetCurrentDtb()

	if opinionData.ParentOpinionId == 0 {
		rootOpinionId = opinionId
	}

	version = 1
	opinionData.CreateDtb = createDtb
	opinionData.Id = opinionId
	opinionData.RootOpinionId = rootOpinionId
	opinionData.UserAccountId = userContext.UserAccountId
	opinionData.Version = version

	compressedOpinion, ok := utils.MarshalZip(opinionData, ctx)
	if !ok {
		return
	}

	opinion := scylladb.Opinion{
		PartitionPeriod: partitionPeriod,
		RootOpinionId:   rootOpinionId,
		OpinionId:       opinionId,
		//AgeSuitability:  opinionData.AgeSuitability,
		//CreateEs:        createDtb,
		//ParentOpinionId: opinionData.ParentOpinionId,
		//PollId:          opinionData.PollId,
		//ThemeId:         opinionData.ThemeId,
		//LocationId:      opinionData.LocationId,
		Version: version,
		//UserAccountId:          userContext.UserAccountId,
		Data:            compressedOpinion.Bytes(),
		InsertProcessed: false,
	}

	if !utils.Insert(insertOpinion, opinion, ctx) {
		return
	}

	if opinionData.ParentOpinionId == 0 {
		// Add a new root_opinion

		opinionDataList := [1]data.Opinion{opinionData}

		compressedRootOpinionData, ok := utils.MarshalZip(opinionDataList, ctx)
		if !ok {
			return
		}

		rootOpinion := scylladb.RootOpinion{}
		rootOpinion.OpinionId = rootOpinionId
		rootOpinion.PollId = opinionData.PollId
		rootOpinion.Version = partitionPeriod
		rootOpinion.CreateEs = createDtb
		rootOpinion.Data = compressedRootOpinionData.Bytes()

		if !utils.Insert(insertRootOpinion, rootOpinion, ctx) {
			return
		}
	}

	if !vespa2.AddOpinion("http://localhost:8086", opinionData, ctx) {
		return
	}

	utils.ReturnId(opinionId, ctx)
}

func UpdateOpinion(ctx *fasthttp.RequestCtx) {
	ctx.SetUserValue("recordType", "opinionUpdate")

	requestBytes := (*ctx).Request.Body()
	opinionData := data.Opinion{}
	if !utils.Unmarshal(requestBytes, &opinionData, ctx) {
		return
	}

	// TODO: verify structure of the data

	var (
		okPreviousPosition  bool
		previousOpinionRows []scylladb.Opinion
		previousOpinion     data.Opinion = data.Opinion{}
		version             int16
		waitGroup           sync.WaitGroup
	)
	userContext := utils.NewParallelUserContext(ctx, opinionData.UserAccountId, &waitGroup)
	if userContext == nil {
		return
	}

	waitGroup.Add(2)
	go utils.GetUserSession(userContext)
	go func() {
		defer waitGroup.Done()

		selectParentPositionQuery := selectPreviousOpinionData.BindMap(qb.M{
			"opinion_id": opinionData.Id,
		})

		okPreviousPosition = utils.Select(selectParentPositionQuery, &previousOpinionRows, ctx)
	}()
	waitGroup.Wait()

	if !okPreviousPosition || !utils.CheckSession(userContext) {
		return
	}

	if len(previousOpinionRows) != 1 {
		log.Printf("Did not find an opinion record with opinion_id: %d\n", opinionData.Id)
		ctx.Error("Internal Server Error", http.StatusInternalServerError)
		return
	}

	if !utils.UnzipUnmarshal(previousOpinionRows[0].Data, previousOpinion, ctx) {
		return
	}

	// TODO: Move this check to Auth rules a la Firebase rules
	if previousOpinion.UserAccountId != (*userContext).UserAccountId {
		log.Printf("Opinion user_id: %d does not match provided user_id: %d\n",
			previousOpinion.UserAccountId, (*userContext).UserAccountId)
		ctx.Error("Internal Server Error", http.StatusInternalServerError)
		return
	}

	// TODO: this can also be done with Firebase style rules
	opinionData.RootOpinionId = previousOpinion.RootOpinionId
	opinionData.PollId = previousOpinion.PollId
	opinionData.ParentOpinionId = previousOpinion.ParentOpinionId
	opinionData.CreateDtb = previousOpinion.CreateDtb

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

	updatePeriod := utils.GetDateMinuteFromEpochSeconds(opinionData.CreateDtb)
	if updatePeriod == partitionPeriod {
		// No, need to create an update record, the original record hasn't yet
		// been picked up by the batch process
		utils.ReturnShortVersion(version, ctx)
		return
	}

	rootOpinionIdMod := int32(opinionData.RootOpinionId % rootOpinionIdModFactor)

	periodUpdatedRootIdSetClause := scylladb.PeriodUpdatedRootOpinionIds{
		RootOpinionIdMod: rootOpinionIdMod,
	}
	updatePeriodUpdatedRootOpinionIdsQuery := updatePeriodUpdatedRootOpinionIds.BindMap(qb.M{
		"partition_period":    partitionPeriod,
		"root_opinion_id_mod": rootOpinionIdMod,
	})
	if !utils.Update(updatePeriodUpdatedRootOpinionIdsQuery, periodUpdatedRootIdSetClause, ctx) {
		return
	}

	opinionUpdate := scylladb.OpinionUpdate{
		PartitionPeriod: partitionPeriod,
		RootOpinionId:   opinionData.RootOpinionId,
		OpinionId:       opinionData.Id,
		Version:         version,
		UpdateProcessed: false,
	}

	if !utils.Insert(insertOpinionUpdate, opinionUpdate, ctx) {
		return
	}

	if !vespa2.UpdateOpinion("http://localhost:8086", opinionData, ctx) {
		return
	}

	utils.ReturnShortVersion(version, ctx)
}

func AddPoll(ctx *fasthttp.RequestCtx) {
	ctx.SetUserValue("recordType", "poll")

	requestBytes := (*ctx).Request.Body()
	pollData := model.Poll{}
	if !utils.Unmarshal(requestBytes, &pollData, ctx) {
		return
	}
	if !utils.IsValidSession(ctx, pollData.UserAccount.Id) {
		return
	}

	pollId, ok := utils.GetSeq(&sequence.PollId, ctx)
	if !ok {
		return
	}

	pollData.Id = pollId
	//_, _, createDtb := utils.GetCurrentDtb()

	//pollData.CreateDtb = createDtb
	pollData.CreatedAt = time.Now().Unix()

	//now := time.Now()
	//secs := now.Unix()
	//fmt.Println(time.Unix(secs, 0))

	compressedPoll, ok := utils.MarshalZip(&pollData, ctx)
	if !ok {
		return
	}

	pollIdMod := int32(pollId % pollIdModFactor)

	poll := scylladb.Poll{
		PollId: pollId,
		//ThemeId:         1,
		//LocationId:      1,
		PollIdMod: pollIdMod,
		//CreateEs:        createDtb,
		//UserAccountId:          pollData.UserAccountId,
		PartitionPeriod: partitionPeriod,
		//AgeSuitability:  0,
		Data:            compressedPoll.Bytes(),
		InsertProcessed: false,
	}

	if !utils.Insert(insertPoll, poll, ctx) {
		return
	}

	if !vespa2.AddPoll("http://localhost:8086", pollData, ctx) {
		return
	}

	utils.ReturnId(pollId, ctx)
}

func main() {
	DB = utils.SetupDb(*crdbPath)
	defer DB.Close()

	sequence.SetupSequences(DB)

	boil.SetDB(DB)

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

	session.SetConsistency(gocql.LocalQuorum)

	utils.SetupAuthQueries(session)

	stmt, names := qb.Insert("opinions").Columns(
		"partition_period",
		"root_opinion_id",
		"opinion_id",
		//"age_suitability",
		//"poll_id",
		//"theme_id",
		//"location_id",
		"version",
		//"parent_opinion_id",
		//"create_es",
		//"user_id",
		"data",
		"insert_processed",
	).ToCql()
	insertOpinion = gocqlx.Query(session.Query(stmt), names)

	stmt, names = qb.Insert("opinion_updates").Columns(
		"partition_period",
		"root_opinion_id",
		"opinion_id",
		"version",
		"update_processed",
	).ToCql()
	insertOpinionUpdate = gocqlx.Query(session.Query(stmt), names)

	stmt, names = qb.Insert("polls").Columns(
		"poll_id",
		"poll_id_mod",
		"partition_period",
		//"theme_id",
		//"location_id",
		//"create_es",
		//"user_id",
		//"age_suitability",
		"data",
		"insert_processed",
	).ToCql()
	insertPoll = gocqlx.Query(session.Query(stmt), names)

	stmt, names = qb.Insert("root_opinions").Columns(
		"opinion_id",
		"poll_id",
		"version",
		"data",
		"create_es",
	).ToCql()
	insertRootOpinion = gocqlx.Query(session.Query(stmt), names)

	stmt, names = qb.Select("opinions").Columns(
		"root_opinion_id",
	).Where(
		qb.Eq("opinion_id"),
	).BypassCache().ToCql()
	selectParentOpinionData = gocqlx.Query(session.Query(stmt), names)

	stmt, names = qb.Select("polls").Columns(
		"poll_id",
	).Where(
		qb.Eq("poll_id"),
	).BypassCache().ToCql()
	selectPollId = gocqlx.Query(session.Query(stmt), names)

	stmt, names = qb.Select("opinions").Columns(
		"create_es",
		"parent_opinion_id",
		"poll_id",
		"root_opinion_id",
		"user_id",
		"version",
	).Where(
		qb.Eq("poll_id"),
		qb.Eq("create_period"),
		qb.Eq("create_es"),
		qb.Eq("opinion_id"),
	).BypassCache().ToCql()
	selectPreviousOpinionData = gocqlx.Query(session.Query(stmt), names)

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

	stmt, names = qb.Update("period_added_to_root_opinion_ids").Set(
		"root_opinion_id",
	).Where(
		qb.Eq("partition_period"),
		qb.Eq("root_opinion_id_mod"),
	).ToCql()
	updatePeriodAddedToRootOpinionIds = gocqlx.Query(session.Query(stmt), names)

	stmt, names = qb.Update("period_updated_root_opinion_ids").Set(
		"root_opinion_id",
	).Where(
		qb.Eq("partition_period"),
		qb.Eq("root_opinion_id_mod"),
	).ToCql()
	updatePeriodUpdatedRootOpinionIds = gocqlx.Query(session.Query(stmt), names)

	r := router.New()
	r.PUT("/add/feedback/:sessionId", AddFeedback)
	r.PUT("/add/feedbackComment/:sessionId", AddFeedbackComment)
	r.PUT("/add/opinion/:sessionPartitionPeriod/:sessionId", AddOpinion)
	r.PUT("/add/poll/:sessionPartitionPeriod/:sessionId", AddPoll)
	r.PUT("/get/feedback/latest/:feedbackTypeId", GetLatestFeedbackForType)
	r.PUT("/get/feedbackComment/latest/:feedbackId", GetLatestFeedbackCommentsForFeedbackId)
	r.PUT("/update/opinion/:sessionPartitionPeriod/:sessionId", UpdateOpinion)

	log.Fatal(fasthttp.ListenAndServe(*addr, r.Handler))
}

func everyPartitionPeriod() {
	partitionPeriod = utils.GetCurrentPartitionPeriod(5)
}

/**
wg                  sync.WaitGroup

	wg.Add(numQueries)
*/
