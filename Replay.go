package ygopro_data

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"unicode/utf16"
	"unicode/utf8"

	"github.com/ulikunitz/xz/lzma"
)

const REPLAY_COMPRESSED_FLAG = 1
const REPLAY_TAG_FLAG = 2
const REPLAY_DECIDED_FLAG = 4
const REPLAY_SINGLE_MODE_FLAG = 8
const REPLAY_UNIFORM_FLAG = 16

const REPLAY_ID_YRP1 = 0x31707279
const REPLAY_ID_YRP2 = 0x32707279

type ReplayHeader struct {
	id, version, flag, seed, hash uint32
	dataSizeRaw                   [4]byte
	props                         [8]byte
	// Extended header fields
	seedSequence           [8]uint32
	headerVersion          uint32
	value1, value2, value3 uint32
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

func ReadReplayFromFile(filename string) (*Replay, error) {
	replay := new(Replay)
	bytes, err := os.ReadFile(filename)
	if err != nil {
		return nil, fmt.Errorf("read replay failed: %w", err)
	}
	headerLength := 32
	if len(bytes) < headerLength {
		return nil, fmt.Errorf("too short replay header: %s", filename)
	}
	replay.header = readReplayHeader(bytes)
	if replay.header.id != REPLAY_ID_YRP1 && replay.header.id != REPLAY_ID_YRP2 {
		return nil, fmt.Errorf("unknown replay version: %s", filename)
	}
	if replay.header.id == REPLAY_ID_YRP2 {
		headerLength = 80
		if len(bytes) < headerLength {
			return nil, fmt.Errorf("too short replay header: %s", filename)
		}
		readReplayHeaderExtended(bytes, replay.header)
	}
	var content []byte
	if replay.header.IsCompressed() {
		content, err = readUncompressedData(bytes[headerLength:], replay.header)
		if err != nil {
			return nil, fmt.Errorf("uncompress replay failed: %w", err)
		}
	} else {
		content = bytes[headerLength:]
	}
	reader := &replayReader{content: content}
	replay.HostName = reader.str(40)
	if replay.header.IsTag() {
		replay.TagHostName = reader.str(40)
		replay.TagClientName = reader.str(40)
	}
	replay.ClientName = reader.str(40)
	replay.StartLP = reader.integer()
	replay.StartHand = reader.integer()
	replay.DrawCount = reader.integer()
	replay.Opt = reader.integer()
	replay.HostDeck = reader.deck()
	if replay.header.IsTag() {
		replay.TagHostDeck = reader.deck()
		replay.TagClientDeck = reader.deck()
	}
	replay.ClientDeck = reader.deck()
	for reader.pos < len(content) && reader.err == nil {
		replay.Responses = append(replay.Responses, reader.response())
	}
	if reader.err != nil {
		return nil, reader.err
	}
	return replay, nil
}

type replayReader struct {
	content []byte
	pos     int
	err     error
}

func (r *replayReader) str(length int) string {
	if r.err != nil {
		return ""
	}
	val, err := readLengthString(r.content, &r.pos, length)
	r.err = err
	return val
}

func (r *replayReader) integer() int {
	if r.err != nil {
		return 0
	}
	val, err := readInteger(r.content, &r.pos)
	r.err = err
	return val
}

func (r *replayReader) deck() Deck {
	if r.err != nil {
		return Deck{}
	}
	deck, err := readDeckFromString(r.content, &r.pos)
	r.err = err
	return deck
}

func (r *replayReader) response() []byte {
	if r.err != nil {
		return nil
	}
	data, err := readResponse(r.content, &r.pos)
	r.err = err
	return data
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

func readReplayHeaderExtended(str []byte, header *ReplayHeader) {
	for i := 0; i < 8; i++ {
		header.seedSequence[i] = binary.LittleEndian.Uint32(str[32+i*4 : 32+(i+1)*4])
	}
	header.headerVersion = binary.LittleEndian.Uint32(str[64:68])
	header.value1 = binary.LittleEndian.Uint32(str[68:72])
	header.value2 = binary.LittleEndian.Uint32(str[72:76])
	header.value3 = binary.LittleEndian.Uint32(str[76:80])
}

func readUncompressedData(str []byte, header *ReplayHeader) ([]byte, error) {
	originBytes := append(header.getLzmaHeader(), str...)
	reader, err := lzma.NewReader(bytes.NewReader(originBytes))
	if err != nil {
		return nil, err
	}
	answer, err := io.ReadAll(reader)
	if err != nil {
		return nil, err
	}
	return answer, nil
}

func readInteger(str []byte, index *int) (int, error) {
	if *index+4 > len(str) {
		return 0, fmt.Errorf("read integer overflow at %d", *index)
	}
	value := binary.LittleEndian.Uint32(str[*index:(*index + 4)])
	*index += 4
	return int(value), nil
}

func readDeckFromString(str []byte, index *int) (Deck, error) {
	deck := Deck{}
	var err error
	deck.Main, err = readDeckPackFromString(str, index)
	if err != nil {
		return Deck{}, err
	}
	deck.Ex, err = readDeckPackFromString(str, index)
	if err != nil {
		return Deck{}, err
	}
	return deck, nil
}

func readDeckPackFromString(str []byte, index *int) ([]int, error) {
	if *index+4 > len(str) {
		return nil, fmt.Errorf("read deck length overflow at %d", *index)
	}
	length := int(binary.LittleEndian.Uint32(str[*index : *index+4]))
	*index += 4
	pack := make([]int, length)
	for i := 0; i < length; i++ {
		if *index+4 > len(str) {
			return nil, fmt.Errorf("read deck card overflow at %d", *index)
		}
		pack[i] = int(binary.LittleEndian.Uint32(str[(*index):(*index + 4)]))
		*index += 4
	}
	return pack, nil
}

func readLengthString(str []byte, index *int, length int) (string, error) {
	if *index+length > len(str) {
		return "", fmt.Errorf("read string overflow at %d", *index)
	}
	value := UTF16BytesToString(str[*index:(*index+length)], binary.LittleEndian)
	*index += length
	return value, nil
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

func readResponse(str []byte, index *int) ([]byte, error) {
	if *index >= len(str) {
		return nil, fmt.Errorf("read response overflow at %d", *index)
	}
	length := int(str[*index])
	*index += 1
	if length > 64 || *index+length > len(str) {
		return nil, fmt.Errorf("invalid response length %d at %d", length, *index)
	} else {
		data := str[(*index):(*index + length)]
		*index += length
		return data, nil
	}
}
