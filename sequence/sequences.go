package sequence

import (
	"database/sql"
)

var (
	OpinionId Sequence
	PollId    Sequence
)

func SetupSequences(db *sql.DB) {
	OpinionId = Sequence{
		CurrentValue: 0,
		Db:           db,
		IncrementBy:  100,
		Max:          0,
		Name:         "OPINION_ID",
	}

	PollId = Sequence{
		CurrentValue: 0,
		Db:           db,
		IncrementBy:  100,
		Max:          0,
		Name:         "POLL_ID",
	}

	numSequences := 2
	seqInitsDone := make(chan bool, numSequences)

	go OpinionId.Init(seqInitsDone)
	go PollId.Init(seqInitsDone)

	numInitializedSequences := 0

	for range seqInitsDone {
		numInitializedSequences++
		if numInitializedSequences == numSequences {
			return
		}
	}
}
