package model

/**
Votecube schema.
*/
type DesignPattern struct {
	CreatedAt int64  `json:"createdAt"`
	Id        int64  `json:"id"`
	Name      string `json:"name"`
	CssClass  string `json:"cssClass"`
}
type Emoji struct {
	CreatedAt int64  `json:"createdAt"`
	Id        int64  `json:"id"`
	Name      string `json:"name"`
	CssClass  string `json:"cssClass"`
}
type Application struct {
	CreatedAt int64  `json:"createdAt"`
	Id        int64  `json:"id"`
	Host      string `json:"host"`
	Port      int64  `json:"port"`
}
type Device struct {
	CreatedAt int64 `json:"createdAt"`
	Id        int64 `json:"id"`
	Hash      int64 `json:"hash"`
}
type UserAccount struct {
	CreatedAt  int64  `json:"createdAt"`
	Id         int64  `json:"id"`
	UserName   string `json:"userName"`
	FirstName  string `json:"firstName"`
	MiddleName string `json:"middleName"`
	LastName   string `json:"lastName"`
	BirthDate  int64  `json:"birthDate"`
}
type Actor struct {
	CreatedAt   int64        `json:"createdAt"`
	Id          int64        `json:"id"`
	Hash        int64        `json:"hash"`
	UserAccount *UserAccount `json:"userAccount"`
	Device      *Device      `json:"device"`
	Application *Application `json:"application"`
}
type CountyTown struct {
	CreatedAt int64   `json:"createdAt"`
	County    *County `json:"county"`
	Town      *Town   `json:"town"`
}
type StateTown struct {
	CreatedAt int64  `json:"createdAt"`
	State     *State `json:"state"`
	Town      *Town  `json:"town"`
}
type State struct {
	CreatedAt  int64        `json:"createdAt"`
	Id         int64        `json:"id"`
	Country    *Country     `json:"country"`
	Name       string       `json:"name"`
	StateTowns *[]StateTown `json:"stateTowns"`
}
type County struct {
	CreatedAt   int64         `json:"createdAt"`
	Id          int64         `json:"id"`
	State       *State        `json:"state"`
	Name        string        `json:"name"`
	CountyTowns *[]CountyTown `json:"countyTowns"`
}
type Town struct {
	CreatedAt int64   `json:"createdAt"`
	Id        int64   `json:"id"`
	County    *County `json:"county"`
	Name      string  `json:"name"`
}
type CountryTown struct {
	CreatedAt int64    `json:"createdAt"`
	Country   *Country `json:"country"`
	Town      *Town    `json:"town"`
}
type Country struct {
	CreatedAt    int64          `json:"createdAt"`
	Id           int64          `json:"id"`
	Continent    *Continent     `json:"continent"`
	Name         string         `json:"name"`
	States       *[]State       `json:"states"`
	CountryTowns *[]CountryTown `json:"countryTowns"`
}
type Continent struct {
	CreatedAt int64      `json:"createdAt"`
	Id        int64      `json:"id"`
	Name      string     `json:"name"`
	Countries *[]Country `json:"countries"`
}
type PollRunContinent struct {
	Id        int64      `json:"id"`
	Continent *Continent `json:"continent"`
	Run       *PollRun   `json:"run"`
}
type PollRunCountry struct {
	Id      int64    `json:"id"`
	Country *Country `json:"country"`
	Run     *PollRun `json:"run"`
}
type PollRunCounty struct {
	Id      int64    `json:"id"`
	Country *County  `json:"country"`
	Run     *PollRun `json:"run"`
}
type PollRunState struct {
	Id    int64    `json:"id"`
	State *State   `json:"state"`
	Run   *PollRun `json:"run"`
}
type PollRunTown struct {
	Id   int64    `json:"id"`
	Town *Town    `json:"town"`
	Run  *PollRun `json:"run"`
}
type PollRun struct {
	CreatedAt          int64               `json:"createdAt"`
	Actor              *Actor              `json:"actor"`
	UserAccount        *UserAccount        `json:"userAccount"`
	Id                 int64               `json:"id"`
	EndDate            int64               `json:"endDate"`
	StartDate          int64               `json:"startDate"`
	PollRevision       *PollRevision       `json:"pollRevision"`
	CreatedAtRevisions *[]PollRevision     `json:"createdAtRevisions"`
	PollContinents     *[]PollRunContinent `json:"pollContinents"`
	PollCountries      *[]PollRunCountry   `json:"pollCountries"`
	PollStates         *[]PollRunState     `json:"pollStates"`
	PollCounties       *[]PollRunCounty    `json:"pollCounties"`
	PollTowns          *[]PollRunTown      `json:"pollTowns"`
}
type Skin struct {
	CreatedAt       int64        `json:"createdAt"`
	Actor           *Actor       `json:"actor"`
	UserAccount     *UserAccount `json:"userAccount"`
	Id              int64        `json:"id"`
	BackgroundColor int64        `json:"backgroundColor"`
	TextColor       int64        `json:"textColor"`
	Parent          *Skin        `json:"parent"`
	Children        *[]Skin      `json:"children"`
}
type PollRevisionFactorPosition struct {
	CreatedAt      int64                         `json:"createdAt"`
	Id             int64                         `json:"id"`
	Axis           string                        `json:"axis"`
	Dir            int64                         `json:"dir"`
	FactorNumber   int64                         `json:"factorNumber"`
	Blue           int64                         `json:"blue"`
	Green          int64                         `json:"green"`
	Red            int64                         `json:"red"`
	OutcomeOrdinal string                        `json:"outcomeOrdinal"`
	Skin           *Skin                         `json:"skin"`
	PollRevision   *PollRevision                 `json:"pollRevision"`
	FactorPosition *FactorPosition               `json:"factorPosition"`
	Parent         *PollRevisionFactorPosition   `json:"parent"`
	Children       *[]PollRevisionFactorPosition `json:"children"`
}
type VoteFactorType struct {
	CreatedAt int64  `json:"createdAt"`
	Id        int64  `json:"id"`
	Value     string `json:"value"`
}
type VoteFactor struct {
	Id            int64                       `json:"id"`
	VoteRevision  *VoteVersion                `json:"voteRevision"`
	Share         string                      `json:"share"`
	PollFactorPos *PollRevisionFactorPosition `json:"pollFactorPos"`
	Type          *VoteFactorType             `json:"type"`
}
type VoteVersion struct {
	CreatedAt   int64         `json:"createdAt"`
	Actor       *Actor        `json:"actor"`
	UserAccount *UserAccount  `json:"userAccount"`
	Id          int64         `json:"id"`
	Vote        *Vote         `json:"vote"`
	Factors     *[]VoteFactor `json:"factors"`
}
type VoteType struct {
	CreatedAt   int64  `json:"createdAt"`
	Id          int64  `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
}
type Vote struct {
	CreatedAt   int64          `json:"createdAt"`
	Actor       *Actor         `json:"actor"`
	UserAccount *UserAccount   `json:"userAccount"`
	Id          int64          `json:"id"`
	Type        int64          `json:"type"`
	Run         *PollRun       `json:"run"`
	Revisions   *[]VoteVersion `json:"revisions"`
}
type Language struct {
	Id   int64  `json:"id"`
	Name string `json:"name"`
}
type FactorOpinionVersionTranslation struct {
	CreatedAt            int64                 `json:"createdAt"`
	Id                   int64                 `json:"id"`
	Original             bool                  `json:"original"`
	Title                string                `json:"title"`
	Text                 string                `json:"text"`
	FactorOpinionVersion *FactorOpinionVersion `json:"factorOpinionVersion"`
	Language             *Language             `json:"language"`
}
type FactorOpinionVersion struct {
	Id                  int64                              `json:"id"`
	PollRevisionOpinion *PollRevisionOpinion               `json:"pollRevisionOpinion"`
	Factor              *Factor                            `json:"factor"`
	Parent              *FactorOpinionVersion              `json:"parent"`
	Children            *[]FactorOpinionVersion            `json:"children"`
	Translations        *[]FactorOpinionVersionTranslation `json:"translations"`
}
type TranslationType struct {
	Id   int64 `json:"id"`
	Code int64 `json:"code"`
}
type OutcomeTranslation struct {
	CreatedAt   int64                 `json:"createdAt"`
	Actor       *Actor                `json:"actor"`
	UserAccount *UserAccount          `json:"userAccount"`
	Id          int64                 `json:"id"`
	Name        string                `json:"name"`
	Outcome     *Outcome              `json:"outcome"`
	Language    *Language             `json:"language"`
	Type        *TranslationType      `json:"type"`
	Parent      *OutcomeTranslation   `json:"parent"`
	Children    *[]OutcomeTranslation `json:"children"`
}
type Outcome struct {
	CreatedAt         int64               `json:"createdAt"`
	Actor             *Actor              `json:"actor"`
	UserAccount       *UserAccount        `json:"userAccount"`
	AgeSuitability    int64               `json:"ageSuitability"`
	Id                int64               `json:"id"`
	ParentTranslation *OutcomeTranslation `json:"parentTranslation"`
	Parent            *Outcome            `json:"parent"`
	Children          *[]Outcome          `json:"children"`
	PollRevisionsA    *[]PollRevision     `json:"pollRevisionsA"`
	PollRevisionsB    *[]PollRevision     `json:"pollRevisionsB"`
}
type OutcomeOpinionVersionTranslation struct {
	CreatedAt             int64                  `json:"createdAt"`
	Id                    int64                  `json:"id"`
	Original              bool                   `json:"original"`
	Title                 string                 `json:"title"`
	Text                  string                 `json:"text"`
	OutcomeOpinionVersion *OutcomeOpinionVersion `json:"outcomeOpinionVersion"`
	Language              *Language              `json:"language"`
}
type OutcomeOpinionVersion struct {
	Id                  int64                               `json:"id"`
	PollRevisionOpinion *PollRevisionOpinion                `json:"pollRevisionOpinion"`
	Outcome             *Outcome                            `json:"outcome"`
	Parent              *OutcomeOpinionVersion              `json:"parent"`
	Children            *[]OutcomeOpinionVersion            `json:"children"`
	Translations        *[]OutcomeOpinionVersionTranslation `json:"translations"`
}
type PollRevisionOpinionVersionTranslation struct {
	CreatedAt      int64                       `json:"createdAt"`
	Id             int64                       `json:"id"`
	Original       bool                        `json:"original"`
	Title          string                      `json:"title"`
	Text           string                      `json:"text"`
	OpinionVersion *PollRevisionOpinionVersion `json:"opinionVersion"`
	Language       *Language                   `json:"language"`
}
type PollRevisionOpinionVersion struct {
	CreatedAt           int64                                    `json:"createdAt"`
	Id                  int64                                    `json:"id"`
	PollRevisionOpinion *PollRevisionOpinion                     `json:"pollRevisionOpinion"`
	Parent              *PollRevisionOpinionVersion              `json:"parent"`
	Children            *[]PollRevisionOpinionVersion            `json:"children"`
	Translations        *[]PollRevisionOpinionVersionTranslation `json:"translations"`
}
type PositionOpinionVersionTranslation struct {
	CreatedAt              int64                   `json:"createdAt"`
	Id                     int64                   `json:"id"`
	Original               bool                    `json:"original"`
	Title                  string                  `json:"title"`
	Text                   string                  `json:"text"`
	PositionOpinionVersion *PositionOpinionVersion `json:"positionOpinionVersion"`
	Language               *Language               `json:"language"`
}
type PositionOpinionVersion struct {
	Id                  int64                                `json:"id"`
	PollRevisionOpinion *PollRevisionOpinion                 `json:"pollRevisionOpinion"`
	FactorPosition      *PollRevisionFactorPosition          `json:"factorPosition"`
	Parent              *PositionOpinionVersion              `json:"parent"`
	Children            *[]PositionOpinionVersion            `json:"children"`
	Translations        *[]PositionOpinionVersionTranslation `json:"translations"`
}
type RatingSetting struct {
	CreatedAt int64    `json:"createdAt"`
	Id        int64    `json:"id"`
	Country   *Country `json:"country"`
	Rating    *Rating  `json:"rating"`
	Key       string   `json:"key"`
	Value     string   `json:"value"`
}
type RatingType struct {
	CreatedAt   int64  `json:"createdAt"`
	Id          int64  `json:"id"`
	Code        string `json:"code"`
	Description string `json:"description"`
}
type Rating struct {
	CreatedAt int64            `json:"createdAt"`
	Id        int64            `json:"id"`
	CssClass  string           `json:"cssClass"`
	Type      *RatingType      `json:"type"`
	Settings  *[]RatingSetting `json:"settings"`
}
type PollRevisionOpinionRating struct {
	CreatedAt           int64                `json:"createdAt"`
	Actor               *Actor               `json:"actor"`
	UserAccount         *UserAccount         `json:"userAccount"`
	Id                  int64                `json:"id"`
	PollRevisionOpinion *PollRevisionOpinion `json:"pollRevisionOpinion"`
	Rating              *Rating              `json:"rating"`
}
type PollRevisionOpinion struct {
	CreatedAt    int64                         `json:"createdAt"`
	Actor        *Actor                        `json:"actor"`
	UserAccount  *UserAccount                  `json:"userAccount"`
	UpdatedAt    int64                         `json:"updatedAt"`
	Id           int64                         `json:"id"`
	PollRevision *PollRevision                 `json:"pollRevision"`
	Run          *PollRun                      `json:"run"`
	Vote         *Vote                         `json:"vote"`
	Ratings      *[]PollRevisionOpinionRating  `json:"ratings"`
	Versions     *[]PollRevisionOpinionVersion `json:"versions"`
	Factors      *[]FactorOpinionVersion       `json:"factors"`
	Outcomes     *[]OutcomeOpinionVersion      `json:"outcomes"`
	Positions    *[]PositionOpinionVersion     `json:"positions"`
}
type PollType struct {
	CreatedAt int64  `json:"createdAt"`
	Id        int64  `json:"id"`
	Value     string `json:"value"`
}
type Theme struct {
	CreatedAt      int64  `json:"createdAt"`
	Id             int64  `json:"id"`
	Name           string `json:"name"`
	AgeSuitability int64  `json:"ageSuitability"`
}
type Poll struct {
	CreatedAt      int64           `json:"createdAt"`
	Actor          *Actor          `json:"actor"`
	UserAccount    *UserAccount    `json:"userAccount"`
	AgeSuitability int64           `json:"ageSuitability"`
	Id             int64           `json:"id"`
	Theme          *Theme          `json:"theme"`
	Type           *PollType       `json:"type"`
	Parent         *Poll           `json:"parent"`
	Children       *[]Poll         `json:"children"`
	Runs           *[]PollRun      `json:"runs"`
	Revisions      *[]PollRevision `json:"revisions"`
}
type PollRevisionRating struct {
	CreatedAt    int64         `json:"createdAt"`
	Actor        *Actor        `json:"actor"`
	UserAccount  *UserAccount  `json:"userAccount"`
	UpdatedAt    int64         `json:"updatedAt"`
	Id           int64         `json:"id"`
	Value        int64         `json:"value"`
	PollRevision *PollRevision `json:"pollRevision"`
	Rating       *Rating       `json:"rating"`
}
type PollRevisionTranslationRating struct {
	CreatedAt   int64                            `json:"createdAt"`
	Actor       *Actor                           `json:"actor"`
	UserAccount *UserAccount                     `json:"userAccount"`
	Id          int64                            `json:"id"`
	Value       int64                            `json:"value"`
	Run         *PollRun                         `json:"run"`
	Rating      *Rating                          `json:"rating"`
	Parent      *PollRevisionTranslationRating   `json:"parent"`
	Child       *[]PollRevisionTranslationRating `json:"child"`
}
type PollRevisionTranslation struct {
	CreatedAt    int64                            `json:"createdAt"`
	Actor        *Actor                           `json:"actor"`
	UserAccount  *UserAccount                     `json:"userAccount"`
	Id           int64                            `json:"id"`
	PollRevision *PollRevision                    `json:"pollRevision"`
	Language     *Language                        `json:"language"`
	Name         string                           `json:"name"`
	Type         *TranslationType                 `json:"type"`
	Parent       *PollRevisionTranslation         `json:"parent"`
	Children     *[]PollRevisionTranslation       `json:"children"`
	Ratings      *[]PollRevisionTranslationRating `json:"ratings"`
}
type PollRevision struct {
	CreatedAt       int64                         `json:"createdAt"`
	Actor           *Actor                        `json:"actor"`
	UserAccount     *UserAccount                  `json:"userAccount"`
	AgeSuitability  int64                         `json:"ageSuitability"`
	Id              int64                         `json:"id"`
	Depth           int64                         `json:"depth"`
	Poll            *Poll                         `json:"poll"`
	CreatedAtRun    *PollRun                      `json:"createdAtRun"`
	OutcomeVersionA *Outcome                      `json:"outcomeVersionA"`
	OutcomeVersionB *Outcome                      `json:"outcomeVersionB"`
	Parent          *PollRevision                 `json:"parent"`
	Children        *[]PollRevision               `json:"children"`
	Ratings         *[]PollRevisionRating         `json:"ratings"`
	FactorPositions *[]PollRevisionFactorPosition `json:"factorPositions"`
	AllTranslations *[]PollRevisionTranslation    `json:"allTranslations"`
	Opinions        *[]PollRevisionOpinion        `json:"opinions"`
}
type FactorTranslation struct {
	CreatedAt    int64                `json:"createdAt"`
	Actor        *Actor               `json:"actor"`
	UserAccount  *UserAccount         `json:"userAccount"`
	Id           int64                `json:"id"`
	Name         string               `json:"name"`
	Factor       *Factor              `json:"factor"`
	Language     *Language            `json:"language"`
	Parent       *FactorTranslation   `json:"parent"`
	Children     *[]FactorTranslation `json:"children"`
	ChildFactors *[]Factor            `json:"childFactors"`
}
type Factor struct {
	CreatedAt             int64              `json:"createdAt"`
	Actor                 *Actor             `json:"actor"`
	UserAccount           *UserAccount       `json:"userAccount"`
	AgeSuitability        int64              `json:"ageSuitability"`
	Id                    int64              `json:"id"`
	CreatedAtPollRevision *PollRevision      `json:"createdAtPollRevision"`
	ParentTranslation     *FactorTranslation `json:"parentTranslation"`
	Parent                *Factor            `json:"parent"`
	Children              *[]Factor          `json:"children"`
	FactorPositions       *[]FactorPosition  `json:"factorPositions"`
}
type PositionTranslation struct {
	CreatedAt   int64                  `json:"createdAt"`
	Actor       *Actor                 `json:"actor"`
	UserAccount *UserAccount           `json:"userAccount"`
	Id          int64                  `json:"id"`
	Name        string                 `json:"name"`
	Position    *Position              `json:"position"`
	Language    *Language              `json:"language"`
	Parent      *PositionTranslation   `json:"parent"`
	Children    *[]PositionTranslation `json:"children"`
}
type Position struct {
	CreatedAt             int64                  `json:"createdAt"`
	Actor                 *Actor                 `json:"actor"`
	UserAccount           *UserAccount           `json:"userAccount"`
	AgeSuitability        int64                  `json:"ageSuitability"`
	Id                    int64                  `json:"id"`
	CreatedAtPollRevision *PollRevision          `json:"createdAtPollRevision"`
	ParentTranslation     *PositionTranslation   `json:"parentTranslation"`
	Parent                *Position              `json:"parent"`
	Children              *[]Position            `json:"children"`
	FactorPositions       *[]FactorPosition      `json:"factorPositions"`
	Translations          *[]PositionTranslation `json:"translations"`
}
type FactorPosition struct {
	CreatedAt   int64        `json:"createdAt"`
	Actor       *Actor       `json:"actor"`
	UserAccount *UserAccount `json:"userAccount"`
	Factor      *Factor      `json:"factor"`
	Position    *Position    `json:"position"`
}
