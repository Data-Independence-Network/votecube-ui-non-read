package main

import (
	"bitbucket.org/votecube/votecube-ui-non-read/sequence"
	"bytes"
	"database/sql"
	"encoding/binary"
	"flag"
	"fmt"
	"log"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/fasthttp/router"
	"github.com/gocql/gocql"
	"github.com/klauspost/compress/gzip"
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
	codeCreated             = 201
	codeBadRequest          = 400
	codeInternalServerError = 500
	insertOpinion           *gocqlx.Queryx
	insertPoll              *gocqlx.Queryx
	insertPollKey           *gocqlx.Queryx
	insertThread            *gocqlx.Queryx
	batchId                 = -1
	gzippers                = sync.Pool{New: func() interface{} {
		return gzip.NewWriter(nil)
	}}
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
		log.Print(err)
		ctx.Response.SetStatusCode(codeInternalServerError)
		return
	}

	now := time.Now()
	createEs := now.Unix()

	opinionId := opinionIdCursor.Next()

	var buf bytes.Buffer
	// https://blog.klauspost.com/gzip-performance-for-go-webservers/
	gz := gzippers.Get().(*gzip.Writer)
	gz.Reset(&buf)

	defer gzippers.Put(gz)
	defer gz.Close()

	if _, err := gz.Write(requestBytes); err != nil {
		log.Printf("Unable to gzip opinion")
		log.Print(err)
		ctx.Response.SetStatusCode(codeInternalServerError)
		return
	}

	opinion := Opinion{
		OpinionId: opinionId,
		PollId:    pollId,
		Date:      dateStamp,
		UserId:    1,
		//CreateDt: time.Now(),
		CreateEs:  createEs,
		Data:      buf.Bytes(),
		Processed: false,
	}

	insert := insertOpinion.BindStruct(opinion)

	if err := insert.Exec(); err != nil {
		log.Printf("AddOpinion: Insert error")
		log.Print(err)
		ctx.Response.SetStatusCode(codeInternalServerError)
		return
	}

	encodeIdAndCreateEs(opinionId, createEs, ctx.Response)
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

	var buf bytes.Buffer
	// https://blog.klauspost.com/gzip-performance-for-go-webservers/
	gz := gzippers.Get().(*gzip.Writer)
	gz.Reset(&buf)

	defer gzippers.Put(gz)
	defer gz.Close()

	if _, err := gz.Write(requestBytes); err != nil {
		log.Printf("Unable to gzip poll")
		log.Print(err)
		ctx.Response.SetStatusCode(codeInternalServerError)
		return
	}

	poll := Poll{
		PollId:   pollId,
		UserId:   1,
		CreateEs: createEs,
		Data:     buf.Bytes(),
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

	encodeIdAndCreateEs(pollId, createEs, ctx.Response)
}

func encodeIdAndCreateEs(id uint64, createEs int64, response fasthttp.Response) {
	idBuffer := new(bytes.Buffer)
	err := binary.Write(idBuffer, binary.LittleEndian, id)
	if err != nil {
		fmt.Println("binary.Write failed:", err)
		response.SetStatusCode(codeInternalServerError)
		return
	}
	idBytes := idBuffer.Bytes()

	var idSignificantBytes []byte
	var byteMask uint8

	if id < 256 {
		idSignificantBytes = idBytes[0:1]
		byteMask = 0
	} else if id < 65536 {
		idSignificantBytes = idBytes[0:2]
		byteMask = 1
	} else if id < 16777216 {
		idSignificantBytes = idBytes[0:3]
		byteMask = 2
	} else if id < 4294967296 {
		idSignificantBytes = idBytes[0:4]
		byteMask = 3
	} else if id < 1099511627776 {
		idSignificantBytes = idBytes[0:5]
		byteMask = 4
	} else if id < 281474976710656 {
		idSignificantBytes = idBytes[0:6]
		byteMask = 5
	} else if id < 72057594037927936 {
		idSignificantBytes = idBytes[0:7]
		byteMask = 6
	} else {
		idSignificantBytes = idBytes
		byteMask = 7
	}

	createEsBuffer := new(bytes.Buffer)
	err = binary.Write(createEsBuffer, binary.LittleEndian, id)
	if err != nil {
		fmt.Println("binary.Write failed:", err)
		response.SetStatusCode(codeInternalServerError)
		return
	}
	createEsBytes := createEsBuffer.Bytes()

	var createEsSignificantBytes []byte

	if createEs < 4294967296 {
		createEsSignificantBytes = createEsBytes[0:4]
	} else {
		createEsSignificantBytes = createEsBytes[0:5]
		byteMask += 8
	}

	response.SetStatusCode(codeCreated)
	response.AppendBody([]byte{byteMask})
	response.AppendBody(idSignificantBytes)
	response.AppendBody(createEsSignificantBytes)
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
	r.PUT("/put/opinion/:pollId", AddOpinion)
	r.PUT("/put/poll", AddPoll)

	log.Fatal(fasthttp.ListenAndServe(*addr, r.Handler))
}
