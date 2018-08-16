package ygopro_data

import (
	"database/sql"
	"fmt"
	_ "github.com/mattn/go-sqlite3"
	"io/ioutil"
	"log"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"os"
)

// SQL 卡片查询指令
const READ_DATA_SQL = "select * from datas join texts on datas.Id == texts.Id where datas.Id == (?)"
const READ_ALL_DATA_SQL = "select * from datas join texts on datas.Id == texts.Id"

// SQL 系列查询指令
const QUERY_SET_SQL = "select Id from datas where (Setcode & 0x0000000000000FFF == (?) or Setcode & 0x000000000FFF0000 == (?) or Setcode & 0x00000FFF00000000 == (?) or Setcode & 0x0FFF000000000000 == (?))"
const QUERY_SUBSET_SQL = "select Id from datas where (Setcode & 0x000000000000FFFF == (?) or Setcode & 0x00000000FFFF0000 == (?) or Setcode & 0x0000FFFF00000000 == (?) or Setcode & 0xFFFF000000000000 == (?))"

// SQL 卡片查询指令
const SEARCH_NAME_ACCURATE_SQL = "select id from texts where name == (?)"
const SEARCH_NAME_SQL = "select id from texts where name like (?)"

type property struct {
	name   string
	text   string
	value  int64
	locale string
}

type Environment struct {
	Cards  map[int]Card
	Locale string
	dbs    []*sql.DB

	attributeNames []string
	raceNames      []string
	typeNames      []string

	Attributes map[string]property
	Races      map[string]property
	Types      map[string]property
	Sets       []Set
}

// 构造函数

var Environments map[string]*Environment = make(map[string]*Environment)
var DatabasePath = filepath.Join(os.Getenv("GOPATH"), "src/github.com/iamipanda/ygopro-data/ygopro-database/locales/")
var LuaPath = filepath.Join(os.Getenv("GOPATH"), "src/github.com/iamipanda/ygopro-data/Constant.lua")

func GetEnvironment(locale string) *Environment {
	if environment, has := Environments[locale]; has {
		return environment
	} else {
		return newEnvironment(locale)
	}
}

func newEnvironment(locale string) (environment *Environment) {
	environment = new(Environment)
	environment.dbs = searchCdb(locale)
	environment.Cards = make(map[int]Card)
	environment.Locale = locale
	environment.loadStringsFile(filepath.Join(DatabasePath, locale, "strings.conf"))
	environment.linkStringsAndConstants()
	environment.linkSetNameToSQL()
	Environments[locale] = environment
	return
}

// 静态初始化（读取 Constants.lua）
var attributeConstants []property = make([]property, 0, 10)
var raceConstants []property = make([]property, 0, 40)
var typeConstants []property = make([]property, 0, 40)

func InitializeStaticEnvironment() {
	loadLuaFile(LuaPath)
	// register_methods
}

func loadLuaFile(filePath string) {
	bytes, err := ioutil.ReadFile(filePath)
	if err != nil {
		log.Fatal()
		fmt.Printf("%v", err)
		return
	}
	stringFile := string(bytes[:])
	loadLuaLines(stringFile)
}

func loadLuaLines(stringFile string) {
	lines := strings.Split(stringFile, "\n")
	for _, line := range lines {
		if strings.HasPrefix(line, "--") {
			continue
		}
		if name, value, err := loadLuaLinePattern(line); err {
			continue
		} else {
			attributeConstants = checkAndAddConstant(name, value, "ATTRIBUTE_", attributeConstants)
			raceConstants = checkAndAddConstant(name, value, "RACE_", raceConstants)
			typeConstants = checkAndAddConstant(name, value, "TYPE_", typeConstants)
		}
	}
}

var luaLineRegex, _ = regexp.Compile(`([A-Z_]+)\s*=\s*0x(\d+)`)

func loadLuaLinePattern(line string) (string, int64, bool) {
	if match := luaLineRegex.FindStringSubmatch(line); match == nil {
		return "", -1, true
	} else {
		value, _ := strconv.ParseInt(match[2], 16, 64)
		return match[1], value, false
	}
}

func checkAndAddConstant(name string, value int64, prefix string, target []property) []property {
	if strings.HasPrefix(name, prefix) {
		name = strings.ToLower(name[len(prefix):])
		target = append(target, property{name: name, value: value})
	}
	return target
}

// 读取 strings 文件步骤
func (environment *Environment) loadStringsFile(filePath string) {
	bytes, err := ioutil.ReadFile(filePath)
	if err != nil {
		fmt.Printf("%v", err)
		return
	}
	stringFile := string(bytes[:])
	environment.loadStringsLines(stringFile)
}

func (environment *Environment) loadStringsLines(string_file string) {
	lines := strings.Split(string_file, "\n")
	for _, line := range lines {
		switch {
		case strings.HasPrefix(line, "!system 10"):
			if systemNumber, text, err := environment.loadStringsLinePattern(line); err {
				continue
			} else {
				switch {
				case isAttributeName(systemNumber):
					environment.attributeNames = append(environment.attributeNames, text)
				case isRaceName(systemNumber):
					environment.raceNames = append(environment.raceNames, text)
				case isTypeName(systemNumber):
					environment.typeNames = append(environment.typeNames, text)
				}
			}
		case strings.HasPrefix(line, "!setname"):
			if setCode, setName, err := environment.loadSetnameLinePattern(line); err {
				continue
			} else {
				environment.Sets = append(environment.Sets, createSet(setCode, setName, environment.Locale))
			}
		}
	}
}

func isAttributeName(systemNumber int64) bool {
	return systemNumber >= 1010 && systemNumber < 1020
}

func isRaceName(systemNumber int64) bool {
	return systemNumber >= 1020 && systemNumber < 1050
}

func isTypeName(systemNumber int64) bool {
	return systemNumber >= 1050 && systemNumber < 1080 && systemNumber != 1053 && systemNumber != 1065
}

var stringsLineReg, _ = regexp.Compile(`!system (\d+) (.+)`)
var setnameLineReg, _ = regexp.Compile(`!setname 0x([0-9a-fA-F]+) (.+)`)

func (Environment) loadStringsLinePattern(line string) (int64, string, bool) {
	if submatches := stringsLineReg.FindStringSubmatch(line); submatches == nil {
		return 0, "", true
	} else {
		value, _ := strconv.ParseInt(submatches[1], 10, 0)
		return value, submatches[2], false
	}
}

func (Environment) loadSetnameLinePattern(line string) (int64, string, bool) {
	if submatches := setnameLineReg.FindStringSubmatch(line); submatches == nil {
		return 0, "", true
	} else {
		value, _ := strconv.ParseInt(submatches[1], 16, 0)
		return value, submatches[2], false
	}
}

// 连接步骤
func (environment *Environment) linkStringsAndConstants() {
	environment.linkStringsAndConstantsPattern(environment.attributeNames, attributeConstants, &environment.Attributes)
	environment.linkStringsAndConstantsPattern(environment.raceNames, raceConstants, &environment.Races)
	environment.linkStringsAndConstantsPattern(environment.typeNames, typeConstants, &environment.Types)

	// Log
}

func (environment *Environment) linkStringsAndConstantsPattern(strings []string, constants []property, target *map[string]property) {
	*target = make(map[string]property)
	for i := 0; i < len(strings) && i < len(constants); i++ {
		constant := constants[i]
		(*target)[constant.name] = property{constant.name, strings[i], constant.value, environment.Locale}
	}
}

// 建立 SQL 连接
func searchCdb(locale string) []*sql.DB {
	if dbPath, err := filepath.Glob(filepath.Join(DatabasePath, locale, "/*.cdb")); err != nil {
		return nil
	} else {
		dbs := make([]*sql.DB, 0)
		for _, path := range dbPath {
			if db, err := sql.Open("sqlite3", path); err != nil {
				fmt.Printf("%v", err)
				continue
			} else {
				dbs = append(dbs, db)
			}
		}
		return dbs
	}
}

// 字段探查
func (environment *Environment) linkSetNameToSQL() {
	for i := range environment.Sets {
		var ids []int
		for _, db := range environment.dbs {
			for _, id := range getIdsBySetCode(db, environment.Sets[i].Code) {
				ids = append(ids, id)
			}
		}
		environment.Sets[i].Ids = ids
	}
}

func getIdsBySetCode(db *sql.DB, setCode int64) []int {
	var sqlQuery string
	if setCode < 0xFFF {
		sqlQuery = QUERY_SET_SQL
	} else {
		sqlQuery = QUERY_SUBSET_SQL
	}
	rows, _ := db.Query(sqlQuery, setCode, setCode<<8, setCode<<16, setCode<<24)
	var ids []int
	var id int
	for rows.Next() {
		rows.Scan(&id)
		ids = append(ids, id)
	}
	return ids
}

// 获取卡片
func (environment *Environment) GetCard(id int) (Card, bool) {
	if card, exist := environment.Cards[id]; exist {
		return card, true
	} else {
		if card, exist = environment.generateCard(id); exist {
			return card, true
		} else {
			return card, false
		}
	}
}

// 根据名称获取卡片
func (environment *Environment) GetNamedCard(name string) (Card, bool) {
	for _, db := range environment.dbs {
		rows, _ := db.Query(SEARCH_NAME_ACCURATE_SQL, name)
		answer := rows.Next()
		if answer {
			var id int
			rows.Scan(&id)
			return environment.GetCard(id)
		}
		rows.Close()
	}
	for _, db := range environment.dbs {
		rows, _ := db.Query(SEARCH_NAME_SQL, "%"+name+"%")
		answer := rows.Next()
		if answer {
			var id int
			rows.Scan(&id)
			return environment.GetCard(id)
		}
		rows.Close()
	}
	return Card{}, false
}

func (environment *Environment) GetNamedCardCached(name string) (Card, bool) {
	for _, card := range environment.Cards {
		if card.Name == name {
			return card, true
		}
	}
	for _, card := range environment.Cards {
		if strings.Contains(card.Name, name) {
			return card, true
		}
	}
	return environment.GetNamedCard(name)
}

func (environment *Environment) GetAllNamedCard(name string) Set {
	if len(name) == 0 {
		return Set{}
	}
	var id int
	var ids []int
	for _, db := range environment.dbs {
		rows, _ := db.Query(SEARCH_NAME_SQL, "%"+name+"%")
		answer := rows.Next()
		for ; answer; answer = rows.Next() {
			rows.Scan(&id)
			ids = append(ids, id)
		}
	}
	return Set{environment.Locale, name, 0,ids, ""}
}

func (environment *Environment) generateCard(id int) (Card, bool) {
	for _, db := range environment.dbs {
		rows, _ := db.Query(READ_DATA_SQL, id)
		answer := rows.Next()
		if answer {
			card := createCardFromData(environment.Locale, rows)
			environment.Cards[card.Id] = card
			return card, true
		}
		rows.Close()
	}
	return Card{}, false
}

func (environment *Environment) LoadAllCards() {
	for _, db := range environment.dbs {
		rows, _ := db.Query(READ_ALL_DATA_SQL)
		for rows.Next() {
			card := createCardFromData(environment.Locale, rows)
			environment.Cards[card.Id] = card
		}
		rows.Close()
	}
}

func (environment *Environment)LoadAllEnvironmentCards() {
	for _, environment := range Environments {
		environment.LoadAllCards()
	}
}

// property query

func (property *property) IsAttribute(card Card) bool {
	return int64(card.Attribute)&property.value > 0
}

func (card Card) IsAttribute(attributeName string) bool {
	attributeName = strings.ToLower(attributeName)
	environment := GetEnvironment(card.Locale)
	if attribute, exist := environment.Attributes[attributeName]; exist {
		return attribute.IsAttribute(card)
	} else {
		return false
	}
}

func (property *property) IsRace(card Card) bool {
	return int64(card.Race)&property.value > 0
}

func (card Card) IsRace(raceName string) bool {
	raceName = strings.ToLower(raceName)
	environment := GetEnvironment(card.Locale)
	if attribute, exist := environment.Races[raceName]; exist {
		return attribute.IsRace(card)
	} else {
		return false
	}
}

func (property *property) IsType(card Card) bool {
	return int64(card.Type)&property.value > 0
}

func (card Card) IsType(typeName string) bool {
	typeName = strings.ToLower(typeName)
	environment := GetEnvironment(card.Locale)
	if attribute, exist := environment.Types[typeName]; exist {
		return attribute.IsType(card)
	} else {
		return false
	}
}
