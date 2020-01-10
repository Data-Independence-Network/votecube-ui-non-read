package main

import (
	"bitbucket.org/votecube/votecube-ui-non-read/sequence"
	"database/sql"
	"flag"
	"log"
	"strconv"
	"strings"
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
	DB                      *sql.DB
	scdbHosts               = flag.String("scdbHosts", "localhost", "TCP address to listen to")
	crdbPath                = flag.String("crdbPath", "root@localhost:26257", "TCP address to listen to")
	addr                    = flag.String("addr", ":8445", "TCP address to listen to")
	cluster                 *gocql.ClusterConfig
	session                 *gocql.Session
	err                     error
	dateStamp               = GetDateStamp()
	codeBadRequest          = 400
	codeInternalServerError = 500
	insertOpinion           *gocqlx.Queryx
	insertPoll              *gocqlx.Queryx
	insertPollKey           *gocqlx.Queryx
	insertThread            *gocqlx.Queryx
	batchId                 = -1
	//compress = flag.Bool("compress", false, "Whether to enable transparent response compression")
)

type Opinion struct {
	OpinionId uint64
	PollId    uint64
	Date      string
	UserId    uint64
	//CreateDt time.Time
	CreateEs  int64
	Data      []byte
	Processed bool
}

type Poll struct {
	PollId   uint64
	UserId   uint64
	CreateEs int64
	Data     []byte
}

type PollKey struct {
	PollId   uint64
	UserId   uint64
	CreateEs int64
	BatchId  int
}

type Thread struct {
	PollId   uint64
	UserId   uint64
	CreateEs int64
	Data     []byte
}

func AddOpinion(ctx *fasthttp.RequestCtx) {
	requestBytes := (*ctx).Request.Body()
	pollId, parseError := strconv.ParseUint(ctx.UserValue("pollId").(string), 0, 64)
	if parseError != nil {
		log.Printf("AddOpinion: Invalid pollId: %s", ctx.UserValue("pollId"))
		ctx.Response.SetStatusCode(codeBadRequest)
		return
	}
	opinionIdCursor, err := sequence.OpinionId.GetCursor(1)
	if err != nil {
		log.Printf("AddOpinion: Unable to access OPINION_ID sequence")
		ctx.Response.SetStatusCode(codeInternalServerError)
		return
	}

	now := time.Now()
	createEs := now.Unix()

	opinion := Opinion{
		OpinionId: opinionIdCursor.Next(),
		PollId:    pollId,
		Date:      dateStamp,
		UserId:    1,
		//CreateDt: time.Now(),
		CreateEs:  createEs,
		Data:      requestBytes,
		Processed: false,
	}

	insert := insertOpinion.BindStruct(opinion)

	if err := insert.Exec(); err != nil {
		log.Printf("AddOpinion: Insert error")
		log.Print(err)
		ctx.Response.SetStatusCode(codeInternalServerError)
		return
	}
}

func AddPoll(ctx *fasthttp.RequestCtx) {
	requestBytes := (*ctx).Request.Body()

	pollIdCursor, err := sequence.PollId.GetCursor(1)
	if err != nil {
		log.Printf("AddPoll: Unable to access POLL_ID sequence")
		ctx.Response.SetStatusCode(codeInternalServerError)
		return
	}

	pollId := pollIdCursor.Next()

	now := time.Now()
	createEs := now.Unix()

	poll := Poll{
		PollId:   pollId,
		UserId:   1,
		CreateEs: createEs,
		Data:     requestBytes,
	}

	batchId = (batchId + 1) % 128

	pollKey := PollKey{
		PollId:   pollId,
		UserId:   1,
		CreateEs: createEs,
		BatchId:  batchId,
	}

	thread := Thread{
		PollId:   pollId,
		UserId:   1,
		CreateEs: createEs,
		Data:     nil,
	}

	insert := insertPoll.BindStruct(poll)
	if err := insert.Exec(); err != nil {
		log.Printf("AddPoll: Insert POLLS error")
		log.Print(err)
		ctx.Response.SetStatusCode(codeInternalServerError)
		return
	}

	insert = insertPollKey.BindStruct(pollKey)
	if err := insert.Exec(); err != nil {
		log.Printf("AddPoll: Insert POLL_KEYS error")
		log.Print(err)
		ctx.Response.SetStatusCode(codeInternalServerError)
		return
	}

	insert = insertThread.BindStruct(thread)
	if err := insert.Exec(); err != nil {
		log.Printf("AddPoll: Insert THREADS error")
		log.Print(err)
		ctx.Response.SetStatusCode(codeInternalServerError)
		return
	}
}

func GetDateStamp() string {
	return time.Now().Format("2006-01-02")
}

func Daily() {
	dateStamp = GetDateStamp()
}

func setupDb() {
	DB, err = sql.Open("postgres", "postgresql://"+*crdbPath+"/votecube?sslmode=disable")

	if err != nil {
		panic(err)
	}

	err = DB.Ping()
	if err != nil {
		panic(err)
	}
}

func main() {
	setupDb()
	sequence.SetupSequences(DB)
	defer DB.Close()

	flag.Parse()
	cron.New(
		cron.WithLocation(time.UTC)).AddFunc("0 0 * * *", Daily)
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

	stmt, names := qb.Insert("opinions").Columns("opinion_id", "poll_id", "date", "user_id", "create_es", "data").ToCql()
	insertOpinion = gocqlx.Query(session.Query(stmt), names)

	stmt, names = qb.Insert("polls").Columns("poll_id", "user_id", "create_es", "data").ToCql()
	insertPoll = gocqlx.Query(session.Query(stmt), names)

	stmt, names = qb.Insert("polls").Columns("poll_id", "user_id", "create_es", "batch_id").ToCql()
	insertPollKey = gocqlx.Query(session.Query(stmt), names)

	stmt, names = qb.Insert("threads").Columns("poll_id", "user_id", "create_es", "data").ToCql()
	insertThread = gocqlx.Query(session.Query(stmt), names)

	r := router.New()
	r.GET("/put/add/opinion/:pollId", AddOpinion)
	r.GET("/put/add/poll", AddPoll)

	log.Fatal(fasthttp.ListenAndServe(*addr, r.Handler))
}
