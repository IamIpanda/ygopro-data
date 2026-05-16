package ygopro_data

import (
	"database/sql"
	_ "embed"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	_ "github.com/mattn/go-sqlite3"
)

//go:embed constant.lua
var embeddedConstantLua []byte

// SQL 卡片查询指令
const READ_DATA_SQL = "select * from datas join texts on datas.Id == texts.Id where datas.Id == (?)"
const READ_ALL_DATA_SQL = "select * from datas join texts on datas.Id == texts.Id"

// SQL 系列查询指令
const QUERY_SET_SQL = "select Id from datas where (Setcode & 0x0000000000000FFF == (?) or Setcode & 0x000000000FFF0000 == (?) or Setcode & 0x00000FFF00000000 == (?) or Setcode & 0x0FFF000000000000 == (?))"
const QUERY_SUBSET_SQL = "select Id from datas where (Setcode & 0x000000000000FFFF == (?) or Setcode & 0x00000000FFFF0000 == (?) or Setcode & 0x0000FFFF00000000 == (?) or Setcode & 0xFFFF000000000000 == (?))"

// SQL 卡片查询指令
const SEARCH_NAME_ACCURATE_SQL = "select id from texts where name == (?)"
const SEARCH_NAME_SQL = "select id from texts where name like (?)"

// Property represents a named constant loaded from the lua constants file.
// It maps a human-readable name (e.g. "dark", "spellcaster") to its integer bitmask value.
// Use IsAttribute, IsRace, and IsType on Card values to test membership.
type Property struct {
	name   string
	text   string
	value  int64
	locale string
}

// Environment provides locale-specific card data, strings, and constants for a
// single language region (e.g. "zh-CN", "en-US"). It reads from .cdb databases
// and a strings.conf file located under a locale directory.
//
// Use GetEnvironment or LoadEnvironment to obtain an instance.
type Environment struct {
	// Cards is a cache of card data keyed by card ID. Populate with LoadAllCards
	// or lazily via GetCard.
	Cards  map[int]Card
	Locale string
	dbs    []*sql.DB

	attributeNames []string
	raceNames      []string
	typeNames      []string

	// Attributes maps lowercase attribute names (e.g. "dark", "light") to Property values.
	Attributes map[string]Property
	// Races maps lowercase race names (e.g. "spellcaster", "dragon") to Property values.
	Races map[string]Property
	// Types maps lowercase type names (e.g. "synchro", "pendulum") to Property values.
	Types map[string]Property
	// Sets holds all set/series definitions for this locale, with card IDs populated.
	Sets []Set
}

// Environments stores all loaded Environment instances keyed by locale string.
// Use GetEnvironment to retrieve or lazily create an entry.
var Environments map[string]*Environment = make(map[string]*Environment)

// Should point to a ygopro-database path. If set, GetEnvironment will auto try locales in that folder.
var DatabasePath string

func ensureLuaLoaded() {
	if !luaLoaded {
		LoadLuaFromBytes(embeddedConstantLua)
	}
}

// GetEnvironment returns the Environment for the given locale, creating it on first access.
// If LoadLuaFile has not been called, the built-in constant.lua is loaded automatically.
// It panics if DatabasePath is not set.
func GetEnvironment(locale string) *Environment {
	ensureLuaLoaded()

	environment, has := Environments[locale]
	if has {
		return environment
	}

	if DatabasePath != "" {
		environment, err := LoadEnvironment(filepath.Join(DatabasePath, "locales", locale), locale)
		if err != nil {
			panic(err)
		}
		return environment
	}
	return nil
}

// LoadEnvironment creates a new Environment from the given locale directory.
//
// path is the full path to the locale directory containing .cdb databases and
// strings.conf files (e.g. "/ygopro-database/locales/zh-CN").
// All files matching *strings.conf* in the directory are loaded and merged.
// locale is the locale identifier (e.g. "zh-CN", "en-US").
//
// The returned Environment is registered in the global Environments map.
func LoadEnvironment(path string, locale string) (*Environment, error) {
	environment := new(Environment)
	environment.Cards = make(map[int]Card)
	environment.Locale = locale
	if err := environment.AppendFolder(path); err != nil {
		return nil, err
	}
	Environments[locale] = environment
	return environment, nil
}

// AttributeConstants holds the raw attribute constants loaded from the lua file
// (e.g. "ATTRIBUTE_DARK" → 0x1). Index-aligned with localized names from strings.conf.
var AttributeConstants []Property

// RaceConstants holds the raw race constants loaded from the lua file
// (e.g. "RACE_SPELLCASTER" → 0x2). Index-aligned with localized names from strings.conf.
var RaceConstants []Property

// TypeConstants holds the raw type constants loaded from the lua file
// (e.g. "TYPE_SYNCHRO" → 0x2000000). Index-aligned with localized names from strings.conf.
var TypeConstants []Property
var luaLoaded bool

type constantsBundle struct {
	attributes []Property
	races      []Property
	types      []Property
}

// LoadLuaFile parses a lua constants file and populates the package-level
// AttributeConstants, RaceConstants, and TypeConstants slices. Must be called once
// before any Environment is created.
//
// If filePath is empty, the built-in constant.lua compiled into the binary is used.
func LoadLuaFile(filePath string) error {
	if filePath == "" {
		return LoadLuaFromBytes(embeddedConstantLua)
	}
	bytes, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("read lua file failed: %w", err)
	}
	return LoadLuaFromBytes(bytes)
}

func LoadLuaFromBytes(data []byte) error {
	if luaLoaded {
		return fmt.Errorf("lua is already loaded")
	}
	stringFile := string(data)
	bundle := loadLuaLines(stringFile)
	AttributeConstants = bundle.attributes
	RaceConstants = bundle.races
	TypeConstants = bundle.types
	luaLoaded = true
	return nil
}

func loadLuaLines(stringFile string) constantsBundle {
	bundle := constantsBundle{
		attributes: make([]Property, 0, 10),
		races:      make([]Property, 0, 40),
		types:      make([]Property, 0, 40),
	}
	lines := strings.Split(stringFile, "\n")
	for _, line := range lines {
		if strings.HasPrefix(line, "--") {
			continue
		}
		name, value, err := loadLuaLinePattern(line)
		if err != nil {
			continue
		}
		bundle.attributes = checkAndAddConstant(name, value, "ATTRIBUTE_", bundle.attributes)
		bundle.races = checkAndAddConstant(name, value, "RACE_", bundle.races)
		bundle.types = checkAndAddConstant(name, value, "TYPE_", bundle.types)
	}
	return bundle
}

var luaLineRegex = regexp.MustCompile(`([A-Z_]+)\s*=\s*0x([0-9a-fA-F]+)`)

func loadLuaLinePattern(line string) (string, int64, error) {
	match := luaLineRegex.FindStringSubmatch(line)
	if match == nil {
		return "", -1, errors.New("not a constant line")
	}
	value, err := strconv.ParseInt(match[2], 16, 64)
	if err != nil {
		return "", -1, fmt.Errorf("parse lua constant failed: %w", err)
	}
	return match[1], value, nil
}

func checkAndAddConstant(name string, value int64, prefix string, target []Property) []Property {
	if strings.HasPrefix(name, prefix) {
		name = strings.ToLower(name[len(prefix):])
		target = append(target, Property{name: name, value: value})
	}
	return target
}

func (environment *Environment) loadStringsFile(filePath string) error {
	bytes, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("read strings file failed: %w", err)
	}
	return environment.loadStringsFromBytes(bytes)
}

func (environment *Environment) loadStringsFromDir(dir string) error {
	matches, err := filepath.Glob(filepath.Join(dir, "*strings.conf*"))
	if err != nil {
		return err
	}
	for _, match := range matches {
		if err := environment.loadStringsFile(match); err != nil {
			return err
		}
	}
	return nil
}

func (environment *Environment) loadStringsFromBytes(data []byte) error {
	stringFile := string(data)
	environment.loadStringsLines(stringFile)
	return nil
}

func (environment *Environment) loadStringsLines(stringFile string) {
	lines := strings.Split(stringFile, "\n")
	for _, line := range lines {
		switch {
		case strings.HasPrefix(line, "!system 10"):
			systemNumber, text, err := environment.loadStringsLinePattern(line)
			if err != nil {
				continue
			}
			switch {
			case isAttributeName(systemNumber):
				environment.attributeNames = append(environment.attributeNames, text)
			case isRaceName(systemNumber):
				environment.raceNames = append(environment.raceNames, text)
			case isTypeName(systemNumber):
				environment.typeNames = append(environment.typeNames, text)
			}
		case strings.HasPrefix(line, "!setname"):
			setCode, setName, err := environment.loadSetnameLinePattern(line)
			if err != nil {
				continue
			}
			environment.Sets = append(environment.Sets, createSet(setCode, setName, environment.Locale))
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

var stringsLineReg = regexp.MustCompile(`!system (\d+) (.+)`)
var setnameLineReg = regexp.MustCompile(`!setname 0x([0-9a-fA-F]+) (.+)`)

func (*Environment) loadStringsLinePattern(line string) (int64, string, error) {
	submatches := stringsLineReg.FindStringSubmatch(line)
	if submatches == nil {
		return 0, "", errors.New("invalid strings line")
	}
	value, err := strconv.ParseInt(submatches[1], 10, 0)
	if err != nil {
		return 0, "", fmt.Errorf("parse strings line failed: %w", err)
	}
	return value, submatches[2], nil
}

func (*Environment) loadSetnameLinePattern(line string) (int64, string, error) {
	submatches := setnameLineReg.FindStringSubmatch(line)
	if submatches == nil {
		return 0, "", errors.New("invalid setname line")
	}
	value, err := strconv.ParseInt(submatches[1], 16, 0)
	if err != nil {
		return 0, "", fmt.Errorf("parse setname line failed: %w", err)
	}
	return value, submatches[2], nil
}

func (environment *Environment) linkStringsAndConstants() {
	environment.linkStringsAndConstantsPattern(environment.attributeNames, AttributeConstants, &environment.Attributes)
	environment.linkStringsAndConstantsPattern(environment.raceNames, RaceConstants, &environment.Races)
	environment.linkStringsAndConstantsPattern(environment.typeNames, TypeConstants, &environment.Types)
}

func (environment *Environment) linkStringsAndConstantsPattern(strings []string, constants []Property, target *map[string]Property) {
	*target = make(map[string]Property)
	for i := 0; i < len(strings) && i < len(constants); i++ {
		constant := constants[i]
		(*target)[constant.name] = Property{constant.name, strings[i], constant.value, environment.Locale}
	}
}

func searchCdb(path string) ([]*sql.DB, error) {
	dbPath, err := filepath.Glob(filepath.Join(path, "*.cdb"))
	if err != nil {
		return nil, err
	}
	dbs := make([]*sql.DB, 0, len(dbPath))
	for _, path := range dbPath {
		db, err := sql.Open("sqlite3", path)
		if err != nil {
			continue
		}
		dbs = append(dbs, db)
	}
	if len(dbs) == 0 {
		return nil, fmt.Errorf("no cdb found in %s", path)
	}
	return dbs, nil
}

// AppendStringsFile loads additional strings from a conf file into an already-loaded
// Environment, then re-links the accumulated names/sets with constants and databases.
// Use this to load extra strings.conf variants after the initial Environment creation.
func (environment *Environment) AppendStringsFile(filePath string) error {
	if err := environment.loadStringsFile(filePath); err != nil {
		return err
	}
	environment.linkStringsAndConstants()
	return environment.linkSetNameToSQL()
}

func (environment *Environment) AppendStringsFolder(dir string) error {
	if err := environment.loadStringsFromDir(dir); err != nil {
		return err
	}
	environment.linkStringsAndConstants()
	return environment.linkSetNameToSQL()
}

func (environment *Environment) AppendFolder(dir string) error {
	dbs, err := searchCdb(dir)
	if err == nil {
		for _, db := range dbs {
			environment.dbs = append(environment.dbs, db)
		}
	}
	if err := environment.loadStringsFromDir(dir); err != nil {
		return err
	}
	environment.linkStringsAndConstants()
	return environment.linkSetNameToSQL()
}

// AppendCdb opens a .cdb file at the given path and appends it to the Environment's
// database list. This allows adding extra card databases to an already-loaded
// Environment without recreating it.
func (environment *Environment) AppendCdb(path string) error {
	db, err := sql.Open("sqlite3", path)
	if err != nil {
		return fmt.Errorf("open cdb failed: %w", err)
	}
	environment.dbs = append(environment.dbs, db)
	return nil
}

func (environment *Environment) linkSetNameToSQL() error {
	for i := range environment.Sets {
		var ids []int
		for _, db := range environment.dbs {
			setIDs, err := getIdsBySetCode(db, environment.Sets[i].Code)
			if err != nil {
				continue
			}
			for _, id := range setIDs {
				ids = append(ids, id)
			}
		}
		environment.Sets[i].Ids = ids
	}
	return nil
}

func getIdsBySetCode(db *sql.DB, setCode int64) ([]int, error) {
	var sqlQuery string
	if setCode < 0xFFF {
		sqlQuery = QUERY_SET_SQL
	} else {
		sqlQuery = QUERY_SUBSET_SQL
	}
	rows, err := db.Query(sqlQuery, setCode, setCode<<16, setCode<<32, setCode<<48)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var ids []int
	var id int
	for rows.Next() {
		if err := rows.Scan(&id); err != nil {
			continue
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return ids, err
	}
	return ids, nil
}

// GetCard returns the Card with the given ID for this environment.
// It first checks the in-memory cache (environment.Cards), then falls back to
// querying the .cdb databases. Results are cached for subsequent calls.
func (environment *Environment) GetCard(id int) (Card, bool) {
	if card, exist := environment.Cards[id]; exist {
		return card, true
	}
	if card, exist := environment.loadCard(id); exist {
		return card, true
	}
	return Card{}, false
}

// GetNamedCard looks up a card by its exact name (or partial match as fallback)
// across all .cdb databases for this environment.
func (environment *Environment) GetNamedCard(name string) (Card, bool) {
	for _, db := range environment.dbs {
		rows, err := db.Query(SEARCH_NAME_ACCURATE_SQL, name)
		if err != nil {
			continue
		}
		answer := rows.Next()
		if answer {
			var id int
			if err := rows.Scan(&id); err != nil {
				rows.Close()
				continue
			}
			rows.Close()
			return environment.GetCard(id)
		}
		rows.Close()
	}
	for _, db := range environment.dbs {
		rows, err := db.Query(SEARCH_NAME_SQL, "%"+name+"%")
		if err != nil {
			continue
		}
		answer := rows.Next()
		if answer {
			var id int
			if err := rows.Scan(&id); err != nil {
				rows.Close()
				continue
			}
			rows.Close()
			return environment.GetCard(id)
		}
		rows.Close()
	}
	return Card{}, false
}

// GetNamedCardCached searches for a card by name, checking the in-memory cache first
// (exact match, then partial match) before falling back to database queries.
// If the cached card is an alias, the original card is returned instead.
func (environment *Environment) GetNamedCardCached(name string) (Card, bool) {
	for _, card := range environment.Cards {
		if card.Name == name {
			if card.Alias > 0 {
				return environment.GetCard(card.Alias)
			}
			return card, true
		}
	}
	for _, card := range environment.Cards {
		if strings.Contains(card.Name, name) {
			if card.Alias > 0 {
				return environment.GetCard(card.Alias)
			}
			return card, true
		}
	}
	return environment.GetNamedCard(name)
}

// GetAllNamedCard returns a Set containing all cards whose names contain the given
// substring, searched across all .cdb databases. Returns an empty Set if name is empty.
func (environment *Environment) GetAllNamedCard(name string) Set {
	if len(name) == 0 {
		return Set{}
	}
	var id int
	var ids []int
	for _, db := range environment.dbs {
		rows, err := db.Query(SEARCH_NAME_SQL, "%"+name+"%")
		if err != nil {
			continue
		}
		answer := rows.Next()
		for answer {
			if err := rows.Scan(&id); err == nil {
				ids = append(ids, id)
			}
			answer = rows.Next()
		}
		rows.Close()
	}
	return Set{environment.Locale, name, 0, ids, ""}
}

func (environment *Environment) loadCard(id int) (Card, bool) {
	for _, db := range environment.dbs {
		rows, err := db.Query(READ_DATA_SQL, id)
		if err != nil {
			continue
		}
		answer := rows.Next()
		if answer {
			card, err := createCardFromData(environment.Locale, rows)
			rows.Close()
			if err != nil {
				continue
			}
			environment.Cards[card.Id] = card
			return card, true
		}
		rows.Close()
	}
	return Card{}, false
}

// LoadAllCards eagerly loads all cards from all .cdb databases into the in-memory cache.
// Subsequent GetCard calls will hit the cache without querying the database.
func (environment *Environment) LoadAllCards() {
	for _, db := range environment.dbs {
		rows, err := db.Query(READ_ALL_DATA_SQL)
		if err != nil {
			continue
		}
		for rows.Next() {
			card, err := createCardFromData(environment.Locale, rows)
			if err != nil {
				continue
			}
			environment.Cards[card.Id] = card
		}
		rows.Close()
	}
}

// LoadAllEnvironmentCards loads all cards for every registered Environment.
func LoadAllEnvironmentCards() {
	for _, environment := range Environments {
		environment.LoadAllCards()
	}
}

func (property *Property) IsAttribute(card Card) bool {
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

func (property *Property) IsRace(card Card) bool {
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

func (property *Property) IsType(card Card) bool {
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
