package main

import (
	"bitbucket.org/votecube/votecube-ui-non-read/sequence"
	"bytes"
	"database/sql"
	"encoding/binary"
	"encoding/json"
	"flag"
	"log"
	"net/http"
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
	DB                  *sql.DB
	scdbHosts           = flag.String("scdbHosts", "localhost", "TCP address to listen to")
	crdbPath            = flag.String("crdbPath", "root@localhost:26257", "TCP address to listen to")
	addr                = flag.String("addr", ":8445", "TCP address to listen to")
	cluster             *gocql.ClusterConfig
	session             *gocql.Session
	err                 error
	dateStamp           = getDateStamp()
	insertOpinion       *gocqlx.Queryx
	insertPoll          *gocqlx.Queryx
	insertThread        *gocqlx.Queryx
	insertOpinionUpdate *gocqlx.Queryx
	batchId             = -1
	gzippers            = sync.Pool{New: func() interface{} {
		return gzip.NewWriter(nil)
	}}
	//compress = flag.Bool("compress", false, "Whether to enable transparent response compression")
)

type Opinion struct {
	OpinionId       uint64
	PollId          uint64
	CreateDate      string
	UserId          uint64
	CreateEs        int64
	UpdateEs        int64
	Version         int32
	Data            []byte
	InsertProcessed bool
}

type OpinionUpdate struct {
	OpinionId       uint64
	PollId          uint64
	UpdateDate      string
	UserId          uint64
	UpdateEs        int64
	Data            []byte
	UpdateProcessed bool
}

type OpinionData struct {
	Id       uint64 `json:"id"`
	PollId   uint64 `json:"pollId"`
	CreateEs int64  `json:"createEs"`
	Text     string `json:"text"`
}

type Poll struct {
	PollId     uint64
	ThemeId    uint64
	LocationId uint32
	UserId     uint64
	Date       string
	CreateEs   int64
	Data       []byte
	BatchId    int
}

type PollData struct {
	Id       uint64 `json:"id"`
	CreateEs int64  `json:"createEs"`
	Title    string `json:"title"`
	Contents string `json:"contents"` // `json:"contents,omitempty"`
}

type Thread struct {
	PollId   uint64
	UserId   uint64
	CreateEs int64
	Data     []byte
}

func AddOpinion(ctx *fasthttp.RequestCtx) {
	ctx.SetUserValue("recordType", "opinion")

	pollId, ok := parseIntParam("pollId", ctx)
	if !ok {
		return
	}
	requestBytes := (*ctx).Request.Body()
	opinionData := OpinionData{}
	if !unmarshal(requestBytes, &opinionData, ctx) {
		return
	}
	opinionId, ok := getSeq(sequence.OpinionId, ctx)
	if !ok {
		return
	}

	opinionData.Id = opinionId
	opinionData.PollId = pollId
	createEs := time.Now().Unix()
	opinionData.CreateEs = createEs

	opinionBytes, ok := marshal(opinionData, ctx)
	if !ok {
		return
	}
	compressedOpinion, ok := zip(opinionBytes, ctx)
	if !ok {
		return
	}

	opinion := Opinion{
		OpinionId:       opinionId,
		PollId:          pollId,
		CreateDate:      dateStamp,
		UserId:          1,
		CreateEs:        createEs,
		UpdateEs:        nil,
		Version:         1,
		Data:            compressedOpinion.Bytes(),
		InsertProcessed: false,
	}

	if !insert(insertOpinion, opinion, ctx) {
		return
	}
	encodeIdAndCreateEs(opinionId, createEs, ctx)
}

func UpdateOpinion(ctx *fasthttp.RequestCtx) {
	ctx.SetUserValue("recordType", "opinionUpdate")

	pollId, ok := parseIntParam("pollId", ctx)
	if !ok {
		return
	}
	opinionId, ok := parseIntParam("opinionId", ctx)
	if !ok {
		return
	}
	createEs, ok := parseIntParam("createEs", ctx)
	if !ok {
		return
	}

}

func AddPoll(ctx *fasthttp.RequestCtx) {
	ctx.SetUserValue("recordType", "poll")

	requestBytes := (*ctx).Request.Body()
	pollData := PollData{}
	if !unmarshal(requestBytes, &pollData, ctx) {
		return
	}
	pollId, ok := getSeq(sequence.PollId, ctx)
	if !ok {
		return
	}

	pollData.Id = pollId
	createEs := time.Now().Unix()
	pollData.CreateEs = createEs

	pollBytes, ok := marshal(pollData, ctx)
	if !ok {
		return
	}
	compressedPoll, ok := zip(pollBytes, ctx)
	if !ok {
		return
	}

	batchId = (batchId + 1) % 128

	poll := Poll{
		PollId:     pollId,
		ThemeId:    1,
		LocationId: 1,
		UserId:     1,
		Date:       dateStamp,
		CreateEs:   createEs,
		Data:       compressedPoll.Bytes(),
		BatchId:    batchId,
	}
	thread := Thread{
		PollId:   pollId,
		UserId:   1,
		CreateEs: createEs,
		Data:     nil,
	}

	if !insert(insertPoll, poll, ctx) {
		return
	}
	if !insert(insertThread, thread, ctx) {
		return
	}
	encodeIdAndCreateEs(pollId, createEs, ctx)
}

func main() {
	setupDb()
	sequence.SetupSequences(DB)
	defer DB.Close()

	flag.Parse()
	cron.New(
		cron.WithLocation(time.UTC)).AddFunc("0 0 * * *", daily)
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
		"create_date",
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
	).Where(qb.Eq("poll_id")).ToCql()
	insertOpinion = gocqlx.Query(session.Query(stmt), names)

	stmt, names = qb.Insert("opinion_updates").Columns(
		"opinion_id",
		"poll_id",
		"update_date",
		"user_id",
		"update_es",
		"data",
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

	r := router.New()
	r.PUT("/put/opinion/:pollId", AddOpinion)
	r.PUT("/update/opinion/:pollId/:createEs/:opinionId", UpdateOpinion)
	r.PUT("/put/poll", AddPoll)

	log.Fatal(fasthttp.ListenAndServe(*addr, r.Handler))
}

func getDateStamp() string {
	return time.Now().Format("20060102")
}

func daily() {
	dateStamp = getDateStamp()
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

func parseIntParam(
	paramName string,
	ctx *fasthttp.RequestCtx,
) (uint64, bool) {
	number, parseError := strconv.ParseUint(ctx.UserValue(paramName).(string), 0, 64)
	if parseError != nil {
		log.Printf("Processing %s - Invalid %s: %s", ctx.UserValue("recordType"), paramName, ctx.UserValue(paramName))
		log.Print(parseError)
		ctx.Error("Internal Server Error", http.StatusInternalServerError)

		return 0, false
	}

	return number, true
}

func unmarshal(
	data []byte,
	v interface{},
	ctx *fasthttp.RequestCtx,
) bool {
	if err := json.Unmarshal(data, v); err != nil {
		log.Printf("Unable to unmarshal %s", ctx.UserValue("recordType"))
		log.Print(err)
		ctx.Error("Internal Server Error", http.StatusInternalServerError)

		return false
	}

	return true
}

func marshal(
	v interface{},
	ctx *fasthttp.RequestCtx,
) ([]byte, bool) {
	dataBytes, err := json.Marshal(v)
	if err != nil {
		log.Printf("Unable to marshal %s", ctx.UserValue("recordType"))
		log.Print(err)
		ctx.Error("Internal Server Error", http.StatusInternalServerError)

		return nil, false
	}

	return dataBytes, true
}

func getSeq(
	sequence sequence.Sequence,
	ctx *fasthttp.RequestCtx,
) (uint64, bool) {
	idCursor, err := sequence.GetCursor(1)
	if err != nil {
		log.Printf("AddOpinion: Unable to access %s sequence", sequence.Name)
		log.Print(err)
		ctx.Error("Internal Server Error", http.StatusInternalServerError)

		return 0, false
	}

	return idCursor.Next(), true
}

func zip(
	dataBytes []byte,
	ctx *fasthttp.RequestCtx,
) (bytes.Buffer, bool) {
	var buf bytes.Buffer
	// https://blog.klauspost.com/gzip-performance-for-go-webservers/
	gz := gzippers.Get().(*gzip.Writer)
	gz.Reset(&buf)

	defer gzippers.Put(gz)

	if _, err := gz.Write(dataBytes); err != nil {
		log.Print("Unable to gzip %s", ctx.UserValue("recordType"))
		log.Print(err)
		gz.Close()
		ctx.Error("Internal Server Error", http.StatusInternalServerError)
		return buf, false
	}
	gz.Close()

	return buf, true
}

func insert(
	preparedInsert *gocqlx.Queryx,
	v interface{},
	ctx *fasthttp.RequestCtx,
) bool {
	return exec("INSERT", preparedInsert, v, ctx)
}

func update(
	preparedInsert *gocqlx.Queryx,
	v interface{},
	ctx *fasthttp.RequestCtx,
) bool {
	return exec("UPDATE", preparedInsert, v, ctx)
}

func exec(
	operationType string,
	preparedInsert *gocqlx.Queryx,
	v interface{},
	ctx *fasthttp.RequestCtx,
) bool {
	statement := preparedInsert.BindStruct(v)

	if err := statement.Exec(); err != nil {
		log.Printf("Error during %s of %s", operationType, ctx.UserValue("recordType"))
		log.Print(err)
		ctx.Error("Internal Server Error", http.StatusInternalServerError)
		return false
	}

	return true
}

func encodeIdAndCreateEs(id uint64, createEs int64, ctx *fasthttp.RequestCtx) {
	idBuffer := new(bytes.Buffer)
	err := binary.Write(idBuffer, binary.LittleEndian, id)
	if err != nil {
		log.Print("binary.Write failed:")
		log.Print(err)
		ctx.Error("Error adding Record", http.StatusInternalServerError)
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
	err = binary.Write(createEsBuffer, binary.LittleEndian, createEs)
	if err != nil {
		log.Print("binary.Write failed")
		log.Print(err)
		ctx.Error("Error adding Record", http.StatusInternalServerError)
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

	/*
		fmt.Println("")
		fmt.Println("id:       %d", id)
		fmt.Println("createEs: %d", createEs)
		fmt.Printf("%d ", byteMask)
		for _, n := range idSignificantBytes {
			fmt.Printf("%d ", n)
		}
		for _, n := range createEsSignificantBytes {
			fmt.Printf("%d ", n)
		}
		fmt.Println("")
	*/

	// https://github.com/valyala/fasthttp/issues/444
	ctx.Response.Reset()
	ctx.SetStatusCode(http.StatusCreated)
	ctx.SetContentType("vcb")
	ctx.Response.AppendBody([]byte{byteMask})
	ctx.Response.AppendBody(idSignificantBytes)
	ctx.Response.AppendBody(createEsSignificantBytes)
}
