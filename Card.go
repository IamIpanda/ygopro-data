package ygopro_data

import (
	"database/sql"
	"fmt"
)

type Card struct {
	Locale                    string
	Id, Ot, Alias             int
	Setcode, Type, Category   int64
	Name, Desc                string
	originLevel               int64
	Race, Attribute, Atk, Def int
	Strs                      [16]string
}

func createCardFromData(locale string, rows *sql.Rows) (Card, error) {
	card := Card{}
	err := rows.Scan(&card.Id, &card.Ot, &card.Alias, &card.Setcode, &card.Type, &card.Atk, &card.Def, &card.originLevel, &card.Race, &card.Attribute, &card.Category, &card.Id, &card.Name, &card.Desc,
		&card.Strs[0], &card.Strs[1], &card.Strs[2], &card.Strs[3], &card.Strs[4], &card.Strs[5], &card.Strs[6], &card.Strs[7], &card.Strs[8], &card.Strs[9], &card.Strs[10], &card.Strs[11], &card.Strs[12], &card.Strs[13], &card.Strs[14], &card.Strs[15])
	if err != nil {
		return Card{}, err
	}
	card.Locale = locale
	return card, nil
}

func (card *Card) IsAlias() bool {
	return card.Alias > 0
}

func (card *Card) IsOcg() bool {
	return card.Ot&1 > 0
}

func (card *Card) IsTcg() bool {
	return card.Ot&2 > 0
}

func (card *Card) IsEx() bool {
	return card.IsType("synchro") || card.IsType("xyz") || card.IsType("fusion") || card.IsType("link")
}

func (card *Card) Level() int {
	return int(card.originLevel % 65536)
}

func (card *Card) PendulumScale() int {
	if card.IsType("pendulum") {
		return int((card.originLevel - card.originLevel%65536) / 65536 / 257)
	} else {
		return -1
	}
}

func (card *Card) LinkMarkers() (markers [9]int) {
	def := card.Def
	for i := 0; i < 9; i++ {
		markers[i] = def % 2
		def = def / 2
	}
	return
}

func (card *Card) LinkNumber() int {
	return card.Level()
}

func (card Card) String() string {
	return fmt.Sprintf("[%v Card] [%v] %v", card.Locale, card.Id, card.Name)
}
