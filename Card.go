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
}

func createCardFromData(locale string, rows *sql.Rows) (card Card) {
	var str1, str2, str3, str4, str5, str6, str7, str8 string
	var str9, str10, str11, str12, str13, str14, str15, str16 string
	rows.Scan(&card.Id, &card.Ot, &card.Alias, &card.Setcode, &card.Type, &card.Atk, &card.Def, &card.originLevel, &card.Race, &card.Attribute, &card.Category, &card.Id, &card.Name, &card.Desc, &str1, &str2, &str3, &str4, &str5, &str6, &str7, &str8, &str9, &str10, &str11, &str12, &str13, &str14, &str15, &str16)
	card.Locale = locale
	return
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
		return int((card.originLevel - card.originLevel%65536) / 65536 / 257);
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