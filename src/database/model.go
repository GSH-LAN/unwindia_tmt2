package database

import (
	"github.com/GSH-LAN/Unwindia_common/src/go/matchservice"
	"github.com/GSH-LAN/Unwindia_tmt2/src/models"
	"github.com/kamva/mgm/v3"
	"time"
)

type Match struct {
	mgm.DefaultModel `bson:",inline"`
	MatchID          string `json:"match_id" bson:"match_id"`
	MatchInfo        matchservice.MatchInfo
	JobState         models.JobState `json:"state"`
	FinishedAt       *time.Time      `json:"finished_at" bson:"finished_at"`
	TMT2MatchId      string          `json:"tmt2_match_id"`
}

func (*Match) CollectionName() string {
	return "tmt2_match"
}
