package ygopro_data

import (
	"encoding/binary"
	"github.com/itchio/lzma"
	"io/ioutil"
	"strings"
	"unicode/utf16"
	"unicode/utf8"
)

const REPLAY_COMPRESSED_FLAG = 1
const REPLAY_TAG_FLAG = 2
const REPLAY_DECIDED_FLAG = 4

type ReplayHeader struct {
	id, version, flag, seed, hash uint32
	dataSizeRaw                   [4]byte
	props                         [8]byte
}

func (header *ReplayHeader) getLzmaHeader() []byte {
	bytes := header.props[0:5] // 6 bytes
	bytes = append(bytes, header.dataSizeRaw[0], header.dataSizeRaw[1], header.dataSizeRaw[2], header.dataSizeRaw[3])
	bytes = append(bytes, 0, 0, 0, 0)
	return bytes
}

func (header *ReplayHeader) IsTag() bool {
	return header.flag&REPLAY_TAG_FLAG > 0
}

func (header *ReplayHeader) IsCompressed() bool {
	return header.flag&REPLAY_COMPRESSED_FLAG > 0
}

func (header *ReplayHeader) IsDecieded() bool {
	return header.flag&REPLAY_DECIDED_FLAG > 0
}

type Replay struct {
	header               *ReplayHeader
	HostName, ClientName string
	StartLP, StartHand   int
	DrawCount, Opt       int
	HostDeck, ClientDeck Deck

	TagHostName, TagClientName string
	TagHostDeck, TagClientDeck Deck

	Responses [][]byte
}

func ReadReplayFromFile(filename string) *Replay {
	replay := new(Replay)
	bytes, err := ioutil.ReadFile(filename)
	if err != nil {
		return nil
	}
	replay.header = readReplayHeader(bytes)
	var content []byte
	if replay.header.IsCompressed() {
		content = readUncompressedData(bytes[32:], replay.header)
	} else {
		content = bytes[32:]
	}
	pos := 0
	replay.HostName = readLengthString(content, &pos, 40)
	if replay.header.IsTag() {
		replay.TagHostName = readLengthString(content, &pos, 40)
		replay.TagClientName = readLengthString(content, &pos, 40)
	}
	replay.ClientName = readLengthString(content, &pos, 40)
	replay.StartLP = readInteger(content, &pos)
	replay.StartHand = readInteger(content, &pos)
	replay.DrawCount = readInteger(content, &pos)
	replay.Opt = readInteger(content, &pos)
	replay.HostDeck = readDeckFromString(content, &pos)
	if replay.header.IsTag() {
		replay.TagHostDeck = readDeckFromString(content, &pos)
		replay.TagClientDeck = readDeckFromString(content, &pos)
	}
	replay.ClientDeck = readDeckFromString(content, &pos)
	for ; pos < len(content); {
		if data, ok := readResponse(content, &pos); ok {
			replay.Responses = append(replay.Responses, data)
		} else {
			break
		}
	}
	return replay
}

func readReplayHeader(str []byte) *ReplayHeader {
	header := new(ReplayHeader)
	header.id = binary.LittleEndian.Uint32(str[0:4])
	header.version = binary.LittleEndian.Uint32(str[4:8])
	header.flag = binary.LittleEndian.Uint32(str[8:12])
	header.seed = binary.LittleEndian.Uint32(str[12:16])
	for i := 0; i < 4; i++ {
		header.dataSizeRaw[i] = str[16+i]
	}
	header.hash = binary.LittleEndian.Uint32(str[20:24])
	for i := 0; i < 8; i++ {
		header.props[i] = str[24+i]
	}
	return header
}

func readUncompressedData(str []byte, header *ReplayHeader) []byte {
	originString := string(header.getLzmaHeader()) + string(str)
	reader := lzma.NewReader(strings.NewReader(originString))
	answer, err := ioutil.ReadAll(reader)
	if err != nil {

	}
	return answer
}

func readInteger(str []byte, index *int) int {
	value := binary.LittleEndian.Uint32(str[*index:(*index + 4)])
	*index += 4
	return int(value)
}

func readDeckFromString(str []byte, index *int) Deck {
	deck := Deck{}
	deck.Main = readDeckPackFromString(str, index)
	deck.Ex = readDeckPackFromString(str, index)
	return deck
}

func readDeckPackFromString(str []byte, index *int) []int {
	length := int(binary.LittleEndian.Uint32(str[*index : *index+4]))
	*index += 4
	pack := make([]int, length)
	for i := 0; i < length; i++ {
		pack[i] = int(binary.LittleEndian.Uint32(str[(*index):(*index + 4)]))
		*index += 4
	}
	return pack
}

func readLengthString(str []byte, index *int, length int) string {
	value := UTF16BytesToString(str[*index:(*index+length)], binary.LittleEndian)
	*index += length
	return value
}

func UTF16BytesToString(b []byte, o binary.ByteOrder) string {
	utf := make([]uint16, (len(b)+1)/2)
	i := 0
	for ; i+1 < len(b); i += 2 {
		utf[i/2] = o.Uint16(b[i:])
		if len(b)/2 < len(utf) {
			utf[len(utf)-1] = utf8.RuneError
		}
		if utf[i/2] == 0 {
			break
		}
	}
	return string(utf16.Decode(utf))
}

func readResponse(str []byte, index *int) ([]byte, bool) {
	length := int(str[*index])
	*index += 1
	if length > 64 || *index + length > len(str) {
		return nil, false
	} else {
		data := str[(*index):(*index+length)]
		*index += length
		return data, true
	}
}