package ygopro_data

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"strconv"
	"strings"
)

const DECK_FILE_HEAD = "#created by lib"
const DECK_FILE_MAIN_FLAG = "#main"
const DECK_FILE_EX_FLAG = "#extra"
const DECK_FILE_SIDE_FLAG = "!side"
const DECK_FILE_NEWLINE = "\n"

type Deck struct {
	Main, Ex, Side, Origin, Cards                                                   []int
	focus                                                                           *[]int
	ClassifiedMain, ClassifiedSide, ClassifiedEx, ClassifiedCards, ClassifiedOrigin map[int]int
}

func (deck Deck) SaveYdk(filename string) {
	file, err := os.Create(filename)
	if err != nil {
		fmt.Fprintln(os.Stderr, "save ydk failed:", filename, "error:", err)
		return
	}
	defer file.Close()
	writer := bufio.NewWriter(file)
	if _, err := writer.WriteString(deck.ToYdk()); err != nil {
		fmt.Fprintln(os.Stderr, "write ydk failed:", filename, "error:", err)
		return
	}
	if err := writer.Flush(); err != nil {
		fmt.Fprintln(os.Stderr, "flush ydk failed:", filename, "error:", err)
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
	file, err := os.Open(filename)
	if err != nil {
		fmt.Fprintln(os.Stderr, "load ydk failed:", filename, "error:", err)
		return Deck{}
	}
	defer file.Close()
	deck := Deck{}
	deck.focus = &deck.Main
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		text := scanner.Text()
		deck.loadYdkLine(text)
	}
	if err := scanner.Err(); err != nil {
		fmt.Fprintln(os.Stderr, "read ydk failed:", filename, "error:", err)
		return deck
	}
	return deck
}

func LoadYdkFromString(string string) Deck {
	string = strings.Replace(string, "\r", "", -1)
	lines := strings.Split(string, "\n")
	deck := Deck{}
	deck.focus = &deck.Main
	for _, line := range lines {
		deck.loadYdkLine(line)
	}
	return deck
}

func (deck *Deck) loadYdkLine(text string) {
	switch {
	case text == DECK_FILE_MAIN_FLAG:
		deck.focus = &deck.Main
	case text == DECK_FILE_SIDE_FLAG:
		deck.focus = &deck.Side
	case text == DECK_FILE_EX_FLAG:
		deck.focus = &deck.Ex
	case strings.HasPrefix(text, "#"):
		return
	case len(text) == 0:
		return
	default:
		value, err := strconv.ParseInt(text, 10, 32)
		if err != nil {
			fmt.Fprintln(os.Stderr, "unknown ydk line:", text, "error:", err)
		} else {
			*deck.focus = append(*deck.focus, int(value))
		}
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
	var newMain []int
	newEx := deck.Ex[0:]
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
	var newMain []int
	newEx := deck.Ex[0:]
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

func (deck *Deck) RemoveAlias(environment *Environment) {
	removePackAlias(deck.Main, environment)
	removePackAlias(deck.Side, environment)
	removePackAlias(deck.Ex, environment)
}

func removePackAlias(pack []int, environment *Environment) {
	for index, id := range pack {
		if card, exist := environment.GetCard(id); exist {
			if card.IsAlias() {
				pack[index] = card.Alias
			}
		}
	}
}

func (deck *Deck) RemoveAliasFromCache(environment *Environment) {
	removePackAliasFromCache(deck.Main, environment)
	removePackAliasFromCache(deck.Side, environment)
	removePackAliasFromCache(deck.Ex, environment)
}

func removePackAliasFromCache(pack []int, environment *Environment) {
	for index, id := range pack {
		if card, exist := environment.Cards[id]; exist {
			if card.IsAlias() {
				pack[index] = card.Alias
			}
		}
	}
}
