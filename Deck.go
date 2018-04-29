package ygopro_data

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
	"bytes"
)

const DECK_FILE_HEAD = "#created by lib"
const DECK_FILE_MAIN_FLAG = "#main"
const DECK_FILE_EX_FLAG = "#extra"
const DECK_FILE_SIDE_FLAG = "!Side"
const DECK_FILE_NEWLINE = "\n"

type Deck struct {
	Main, Ex, Side, Origin, Cards                                                   []int
	focus                                                                           *[]int
	ClassifiedMain, ClassifiedSide, ClassifiedEx, ClassifiedCards, ClassifiedOrigin map[int]int
}

func (deck Deck) SaveYdk(filename string) {
	file, _ := os.Create(filename)
	defer file.Close()
	writer := bufio.NewWriter(file)
	writer.WriteString(DECK_FILE_HEAD + DECK_FILE_NEWLINE)
	writer.WriteString(DECK_FILE_MAIN_FLAG + DECK_FILE_NEWLINE)
	for _, id := range deck.Main {
		writer.WriteString(strconv.Itoa(id) + DECK_FILE_NEWLINE)
	}
	writer.WriteString(DECK_FILE_SIDE_FLAG + DECK_FILE_NEWLINE)
	for _, id := range deck.Side {
		writer.WriteString(strconv.Itoa(id) + DECK_FILE_NEWLINE)
	}
	writer.WriteString(DECK_FILE_EX_FLAG + DECK_FILE_NEWLINE)
	for _, id := range deck.Ex {
		writer.WriteString(strconv.Itoa(id) + DECK_FILE_NEWLINE)
	}
}

func (deck Deck) ToYdk() string {
	var writer bytes.Buffer
	writer.WriteString(DECK_FILE_HEAD + DECK_FILE_NEWLINE)
	writer.WriteString(DECK_FILE_MAIN_FLAG + DECK_FILE_NEWLINE)
	for _, id := range deck.Main {
		writer.WriteString(strconv.Itoa(id) + DECK_FILE_NEWLINE)
	}
	writer.WriteString(DECK_FILE_SIDE_FLAG + DECK_FILE_NEWLINE)
	for _, id := range deck.Side {
		writer.WriteString(strconv.Itoa(id) + DECK_FILE_NEWLINE)
	}
	writer.WriteString(DECK_FILE_EX_FLAG + DECK_FILE_NEWLINE)
	for _, id := range deck.Ex {
		writer.WriteString(strconv.Itoa(id) + DECK_FILE_NEWLINE)
	}
	return writer.String()
}

func LoadYdk(filename string) Deck {
	file, _ := os.Open(filename)
	defer file.Close()
	deck := Deck{}
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		text := scanner.Text()
		deck.loadYdkLine(text)
	}
	if err := scanner.Err(); err != nil {
		fmt.Fprintln(os.Stderr, "reading standard input:", err)
	}
	return deck
}

func LoadYdkFromString(string string) Deck {
	string = strings.Replace(string, "\r", "", -1)
	lines := strings.Split(string, "\n")
	deck := Deck{}
	for _, line := range lines {
		deck.loadYdkLine(line)
	}
	return deck
}

func (deck *Deck) loadYdkLine(text string) {
	deck.focus = &deck.Main
	switch {
	case strings.HasPrefix(text, "#"):
		return
	case text == DECK_FILE_MAIN_FLAG:
		deck.focus = &deck.Main
	case text == DECK_FILE_SIDE_FLAG:
		deck.focus = &deck.Side
	case text == DECK_FILE_EX_FLAG:
		deck.focus = &deck.Ex
	default:
		value, _ := strconv.ParseInt(text, 10, 32)
		*deck.focus = append(*deck.focus, int(value))
	}
}

func (deck *Deck) Summary() {
	deck.Origin = append(deck.Main, deck.Ex...)
	deck.Cards = append(deck.Origin, deck.Side...)
}

func (deck *Deck) Classify() {
	deck.ClassifiedMain = classifyPack(deck.Main)
	deck.ClassifiedSide = classifyPack(deck.Side)
	deck.ClassifiedEx = classifyPack(deck.Ex)
	deck.ClassifiedOrigin = classifyPack(deck.Origin)
	deck.ClassifiedCards = classifyPack(deck.Cards)
}

func classifyPack(pack []int) map[int]int {
	hash := make(map[int]int)
	if pack == nil {
		return hash
	}
	for _, card := range pack {
		num, exist := hash[card]
		if exist {
			hash[card] = num + 1
		} else {
			hash[card] = 1
		}
	}
	return hash
}

func (deck *Deck) SeparateExFromMain(environment *Environment) {
	var newMain, newEx []int
	for _, id := range deck.Main {
		if card, exist := environment.GetCard(id); exist {
			if card.IsEx() {
				newEx = append(newEx, id)
			} else {
				newMain = append(newMain, id)
			}
		}
	}
	deck.Main = newMain
	deck.Ex = newEx
}

func (deck *Deck) SeparateExFromMainFromCache(environment *Environment) {
	var newMain, newEx []int
	for _, id := range deck.Main {
		if card, exist := environment.Cards[id]; exist {
			if card.IsEx() {
				newEx = append(newEx, id)
			} else {
				newMain = append(newMain, id)
			}
		}
	}
	deck.Main = newMain
	deck.Ex = newEx
}
