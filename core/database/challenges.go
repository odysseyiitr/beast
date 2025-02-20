package database

import (
	"bytes"
	"crypto/sha256"
	"errors"
	"fmt"
	"html/template"
	"io/ioutil"
	"path/filepath"
	"time"

	"github.com/sdslabs/beastv4/core"
	tools "github.com/sdslabs/beastv4/templates"
	_ "gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// The `challenges` table has the following columns
// name
// author
// format
// container_id
// image_id
// status
//
// Some hooks needs to be attached to these database transaction, and on the basis of
// the type of the transaction that is performed on the challenge table, we need to
// perform some action.
//
// Use gorm hooks for these purpose, currently the following hooks are
// implemented.
// * AfterUpdate
// * AfterCreate
// * AfterSave
// * AfterDelete
//
// All these hooks are used for generating the access shell script for the challenge
// to the challenge author
type Challenge struct {
	gorm.Model

	Name            string `gorm:"not null;type:varchar(64);unique"`
	Flag            string `gorm:"not null;type:text"`
	Type            string `gorm:"type:varchar(64)"`
	Sidecar         string `gorm:"type:varchar(64)"`
	Hints           string `gorm:"type:text"`
	Assets          string `gorm:"type:text"`
	AdditionalLinks string `gorm:"type:text"`
	Description     string `gorm:"type:text"`
	Format          string `gorm:"not null"`
	ContainerId     string `gorm:"size:64;unique"`
	ImageId         string `gorm:"size:64;unique"`
	Status          string `gorm:"not null;default:'Undeployed'"`
	AuthorID        uint   `gorm:"not null"`
	HealthCheck     uint   `gorm:"not null;default:1"`
	Points          uint   `gorm:"default:0"`
	MaxPoints       uint   `gorm:"default:0"`
	MinPoints       uint   `gorm:"default:0"`
	Ports           []Port
	Tags            []*Tag  `gorm:"many2many:tag_challenges;"`
	Users           []*User `gorm:"many2many:user_challenges;"`
}

type UserChallenges struct {
	CreatedAt   time.Time
	UserID      uint
	ChallengeID uint
}

// Create an entry for the challenge in the Challenge table
// It returns an error if anything wrong happen during the
// transaction.
func CreateChallengeEntry(challenge *Challenge) error {

	DBMux.Lock()
	defer DBMux.Unlock()

	tx := Db.Begin()

	if tx.Error != nil {
		return fmt.Errorf("Error while starting transaction", tx.Error)
	}

	if err := tx.FirstOrCreate(challenge, *challenge).Error; err != nil {
		tx.Rollback()
		return err
	}

	return tx.Commit().Error
}

// Query challenges table to get all the entries in the table
func QueryAllChallenges() ([]Challenge, error) {
	var challenges []Challenge

	DBMux.Lock()
	defer DBMux.Unlock()

	tx := Db.Preload("Ports").Preload("Tags").Find(&challenges)

	if errors.Is(tx.Error, gorm.ErrRecordNotFound) {
		return nil, nil
	}

	return challenges, tx.Error
}

// Queries all the challenges entries where the column represented by key
// have the value in value.
func QueryChallengeEntries(key string, value string) ([]Challenge, error) {
	queryKey := fmt.Sprintf("%s = ?", key)

	var challenges []Challenge

	DBMux.Lock()
	defer DBMux.Unlock()

	tx := Db.Preload("Tags").Preload("Ports").Where(queryKey, value).Find(&challenges)
	if errors.Is(tx.Error, gorm.ErrRecordNotFound) {
		return nil, nil
	}

	if tx.Error != nil {
		return challenges, tx.Error
	}

	return challenges, nil
}

// Queries all the challenges entries where the column matches
func QueryChallengeEntriesMap(whereMap map[string]interface{}) ([]Challenge, error) {

	var challenges []Challenge

	DBMux.Lock()
	defer DBMux.Unlock()

	tx := Db.Where(whereMap).Find(&challenges)
	if errors.Is(tx.Error, gorm.ErrRecordNotFound) {
		return nil, nil
	}

	if tx.Error != nil {
		return challenges, tx.Error
	}

	return challenges, nil
}

// Using the column value in key and value in value get the first
// result of the query.
func QueryFirstChallengeEntry(key string, value string) (Challenge, error) {
	challenges, err := QueryChallengeEntries(key, value)
	if err != nil {
		return Challenge{}, err
	}

	if len(challenges) == 0 {
		return Challenge{}, nil
	}

	return challenges[0], nil
}

// Update an entry for the challenge in the Challenge table
func UpdateChallenge(chall *Challenge, m map[string]interface{}) error {

	DBMux.Lock()
	defer DBMux.Unlock()

	return Db.Omit(clause.Associations).Where("id = ?", chall.ID).Model(chall).Updates(m).Error
}

// This function updates a challenge entry in the database, whereMap is the map
// which contains key value pairs of column and values to filter out the record
// to update. chall is the Challenge variable with the values to update with.
// This function returns any error that might occur while updating the challenge
// entry which includes the error in case the challenge already does not exist in the
// database.
func BatchUpdateChallenge(whereMap map[string]interface{}, chall Challenge) error {
	var challenge Challenge

	DBMux.Lock()
	defer DBMux.Unlock()

	tx := Db.Where(whereMap).First(&challenge)
	if errors.Is(tx.Error, gorm.ErrRecordNotFound) {
		return fmt.Errorf("No challenge entry to update : WhereClause : %s", whereMap)
	}

	if tx.Error != nil {
		return tx.Error
	}

	// Update the found entry
	tx = Db.Model(&challenge).Updates(chall)

	return tx.Error
}

//Get Related Tags
func GetRelatedTags(challenge *Challenge) ([]Tag, error) {
	var tags []Tag

	DBMux.Lock()
	defer DBMux.Unlock()

	if err := Db.Model(challenge).Association("Tags").Error; err != nil {
		return tags, err
	}

	return tags, nil
}

//Get Related Users
func GetRelatedUsers(challenge *Challenge) ([]User, error) {
	var users []User

	DBMux.Lock()
	defer DBMux.Unlock()

	if err := Db.Model(challenge).Association("Users").Find(&users); err != nil {
		return users, err
	}

	return users, nil
}

func DeleteChallengeEntry(challenge *Challenge) error {
	DBMux.Lock()
	defer DBMux.Unlock()

	tx := Db.Begin()

	if tx.Error != nil {
		return fmt.Errorf("Error while starting transaction : %s", tx.Error)
	}

	if err := tx.Unscoped().Delete(challenge).Error; err != nil {
		tx.Rollback()
		return err
	}

	return tx.Commit().Error
}

// Query challenges table to get all the entries in the table
func QueryAllSubmissions() ([]UserChallenges, error) {
	var userChallenges []UserChallenges

	DBMux.Lock()
	defer DBMux.Unlock()

	tx := Db.Find(&userChallenges)

	if errors.Is(tx.Error, gorm.ErrRecordNotFound) {
		return nil, nil
	}

	return userChallenges, tx.Error
}

// QuerySubmissions queries all challenge where column matches
func QuerySubmissions(whereMap map[string]interface{}) ([]UserChallenges, error) {
	var userChallenges []UserChallenges

	DBMux.Lock()
	defer DBMux.Unlock()

	tx := Db.Where(whereMap).Find(&userChallenges)
	if errors.Is(tx.Error, gorm.ErrRecordNotFound) {
		return nil, nil
	}

	if tx.Error != nil {
		return userChallenges, tx.Error
	}

	return userChallenges, nil
}

func SaveFlagSubmission(user_challenges *UserChallenges) error {
	DBMux.Lock()
	defer DBMux.Unlock()

	tx := Db.Begin()

	if tx.Error != nil {
		return fmt.Errorf("Error while saving record", tx.Error)
	}

	if err := tx.FirstOrCreate(user_challenges, *user_challenges).Error; err != nil {
		tx.Rollback()
		return err
	}
	return tx.Commit().Error
}

//hook after update of challenge
func (challenge *Challenge) AfterUpdate(tx *gorm.DB) error {
	iFace, _ := tx.InstanceGet("gorm:update_attrs")
	if iFace == nil {
		return nil
	}
	updatedAttr := iFace.(map[string]interface{})
	if _, ok := updatedAttr["container_id"]; ok {
		var users []*User
		Db.Model(challenge).Association("Users")
		go updateScripts(users)
	}
	return nil
}

//hook after create of challenge
func (challenge *Challenge) AfterCreate(tx *gorm.DB) error {
	var users []*User
	Db.Model(challenge).Association("Users")
	go updateScripts(users)

	return nil
}

//hook after deleting the challenge
func (challenge *Challenge) AfterDelete(tx *gorm.DB) error {
	var users []*User
	Db.Model(challenge).Association("Users")
	go updateScripts(users)
	return nil
}

type ScriptFile struct {
	User       string
	Challenges map[string]string
}

//updates users' script
func updateScripts(users []*User) {
	for _, user := range users {
		go updateScript(user)
	}
}

//updates user script
func updateScript(user *User) error {

	time.Sleep(3 * time.Second)

	SHA256 := sha256.New()
	SHA256.Write([]byte(user.Email))
	scriptPath := filepath.Join(core.BEAST_GLOBAL_DIR, core.BEAST_SCRIPTS_DIR, fmt.Sprintf("%x", SHA256.Sum(nil)))
	challs, err := GetRelatedChallenges(user)
	if err != nil {
		return fmt.Errorf("Error while getting related challenges : %v", err)
	}

	mapOfChall := make(map[string]string)

	for _, chall := range challs {
		mapOfChall[chall.Name] = chall.ContainerId
	}

	data := ScriptFile{
		User:       user.Name,
		Challenges: mapOfChall,
	}

	var script bytes.Buffer
	scriptTemplate, err := template.New("script").Parse(tools.SSH_LOGIN_SCRIPT_TEMPLATE)
	if err != nil {
		return fmt.Errorf("Error while parsing script template :: %s", err)
	}

	err = scriptTemplate.Execute(&script, data)
	if err != nil {
		return fmt.Errorf("Error while executing script template :: %s", err)
	}

	return ioutil.WriteFile(scriptPath, script.Bytes(), 0755)
}
