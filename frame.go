package http2

import (
	"encoding/binary"
	"fmt"
)

// HTTP/2 帧头部
// 内存布局
//
//	0 1 2 3 4 5 6 7 0 1 2 3 4 5 6 7 0 1 2 3 4 5 6 7 0 1 2 3 4 5 6 7
//
// +----------------+---------------+---------------+--------------+
// |                   Length (24)                 |    Type(8)   |
// +----------------+---------------+---------------+--------------+
// |    Flags(8)   |R| 			      StreamId (23)               |
// |  StreamId(8)  |
type FrameHeader struct {
	Length   uint32 // 24位长度
	Type     uint8
	Flags    uint8
	StreamID uint32 // 31位流ID
}

// 通用帧接口
type Frame interface {
	Header() FrameHeader
	Serialize() ([]byte, error)
}

// 数据帧
type DataFrame struct {
	FrameHeader
	PadLen uint8
	Data   []byte
}

// 头部帧
type HeadersFrame struct {
	FrameHeader
	PadLen           uint8
	Exclusive        bool
	StreamDependency uint32
	Weight           uint8
	HeaderBlock      []byte
}

// 设置帧
type SettingsFrame struct {
	FrameHeader
	Settings []Setting
}

type Setting struct {
	ID    uint16
	Value uint32
}

// Ping帧
type PingFrame struct {
	FrameHeader
	Data [8]byte
}

// 窗口更新帧
type WindowUpdateFrame struct {
	FrameHeader
	WindowSizeIncrement uint32
}

// =========================== FrameHeader ===========================
// HeaderFrame 序列化
func (h *FrameHeader) Serialize() []byte {
	buf := make([]byte, 9)
	// 头部3字节
	buf[0] = byte(h.Length >> 16)
	buf[1] = byte(h.Length >> 8)
	buf[2] = byte(h.Length)

	// 帧类型1字节
	buf[3] = h.Type
	// 标志位1字节
	buf[4] = h.Flags
	// 流ID
	binary.BigEndian.PutUint32(buf[5:9], h.StreamID&0x7FFFFFFF)
	return buf
}

// 解析帧头部
func ParseFrameHeader(data []byte) (*FrameHeader, error) {
	if len(data) < 9 {
		return &FrameHeader{}, fmt.Errorf("FRAME_SIZE_ERROR: frame header must be 9 bytes, got %d", len(data))
	}
	return &FrameHeader{
		Length:   uint32(data[0])<<16 | uint32(data[1])<<8 | uint32(data[2]),
		Type:     data[3],
		Flags:    data[4],
		StreamID: binary.BigEndian.Uint32(data[5:9]) & (1<<31 - 1),
	}, nil
}

func (h *FrameHeader) Header() FrameHeader {
	return *h
}

// =========================== HeadersFrame ===========================
// HeaderFrame 序列化
func (f *HeadersFrame) Serialize() ([]byte, error) {
	if f.StreamID == 0 {
		return nil, fmt.Errorf("HEADERS_FRAME_ERROR: frame must have non-zero stream ID")
	}
	// 计算载荷长度
	payloadLength := uint32(len(f.HeaderBlock))

	// 处理填充
	if hasPad(f.Flags) {
		if f.PadLen > 255 {
			return nil, fmt.Errorf("HEADERS_FRAME_ERROR: padLen is out of range: %d", f.PadLen)
		}
		payloadLength = 1 + uint32(f.PadLen)
	}

	// 处理优先级
	if hasPriority(f.Flags) {
		payloadLength += 5
	}

	if payloadLength > 0xFFFFFF {
		return nil, fmt.Errorf("HEADERS_FRAME_ERROR: payloadLength is out of range: %d", payloadLength)
	}

	// 更新枕头
	f.Length = payloadLength
	header := f.FrameHeader.Serialize()

	// 预分配完整帧
	offset := len(header)
	frameLength := offset + int(payloadLength)
	frame := make([]byte, frameLength)
	copy(frame, header)

	// 写入填充长度
	if hasPad(f.Flags) {
		frame[offset] = f.PadLen
		offset++
	}

	// 写入优先级信息
	if hasPriority(f.Flags) {
		var dep uint32
		if f.Exclusive {
			dep = f.StreamDependency | 0x80000000 // 设置最高优先级
		} else {
			dep = f.StreamDependency | 0x7FFFFFFF
		}
		binary.BigEndian.PutUint32(frame[offset:offset+4], dep)
		offset += 4
		frame[offset] = f.Weight
		offset++
	}

	// 写入头部块
	copy(frame[offset:], f.HeaderBlock)

	// 填充区已经是0值，无需操作

	return frame, nil
}

// 解析 HeadersFrame
func ParseHeadersFrame(header *FrameHeader, payload []byte) (*HeadersFrame, error) {
	frame := &HeadersFrame{
		FrameHeader: *header,
	}
	offset := 0

	// 处理填充
	if hasPad(header.Flags) {
		if len(payload) < 1 {
			return nil, fmt.Errorf("HEADERS_FRAME_ERROR: pad is required")
		}
		frame.PadLen = payload[0]
		offset++
	}

	// 处理优先级
	if hasPriority(header.Flags) {
		if len(payload) < offset+5 {
			return nil, fmt.Errorf("HEADERS_FRAME_ERROR: priority is required")
		}
		dep := binary.BigEndian.Uint32(payload[offset : offset+4])
		frame.Exclusive = dep&0x80000000 != 0     // 1位
		frame.StreamDependency = dep & 0x7FFFFFFF // 31位
		offset += 4                               // Exclusive+StreamDependency共32位4字节
		frame.Weight = payload[offset]
		offset++
	}

	// 处理头部块（排除填充）
	headerBlockLength := len(payload) - offset - int(frame.PadLen)
	if headerBlockLength < 0 {
		return nil, fmt.Errorf("HEADERS_FRAME_ERROR: header block length must not be negative")
	}
	frame.HeaderBlock = payload[offset : offset+headerBlockLength]
	return frame, nil
}

func hasPad(flags uint8) bool {
	return flags&FlagPadded != 0
}

func hasPriority(flags uint8) bool {
	return flags&FlagPriority != 0
}

// =========================== DataFrame ===========================
// DataFram 序列化
func (f *DataFrame) Serialize() ([]byte, error) {
	// 验证数据
	if f.StreamID == 0 {
		return nil, fmt.Errorf("DATA_FRAME_ERROR: StreamID must be greater than zero")
	}

	// 计算载荷长度
	var payloadLength uint32

	if f.Flags&FlagPadded != 0 {
		// 验证填充长度
		if f.PadLen > 255 {
			return nil, fmt.Errorf("DATA_FRAME_ERROR: PadLen must be <= 255")
		}
		payloadLength = 1 + uint32(len(f.Data)) + uint32(f.PadLen)

	} else {
		payloadLength = uint32(len(f.Data))
	}

	// 验证总长度不超过24位限制
	if payloadLength > 0xFFFFFF {
		return nil, fmt.Errorf("")
	}
	// 更新帧头部长度
	f.Length = payloadLength

	// 组装成完整帧
	header := f.FrameHeader.Serialize()
	offset := len(header)
	frame := make([]byte, offset+int(payloadLength))
	copy(frame, header)
	if f.Flags&FlagPadded != 0 {
		frame[offset] = f.PadLen
		offset++
	}
	copy(frame[offset:], f.Data)
	// 填充区已经是0值，无需操作

	return frame, nil
}

func ParseDataFrame(header *FrameHeader, payload []byte) (*DataFrame, error) {
	if header.Type != FrameData {
		return nil, fmt.Errorf("DATA_FRAME_ERROR: expected frame type %d, got %d", FrameData, header.Type)
	}
	frame := &DataFrame{
		FrameHeader: *header,
	}
	// 解析载荷
	offset := 0

	// 处理填充
	if header.Flags&FlagPadded != 0 {
		if len(payload) < 1 {
			return nil, fmt.Errorf("DATA_FRAME_ERROR: pad is required")
		}
		frame.PadLen = payload[0]
		offset += 1

		// 验证填充长度
		if int(frame.PadLen) > len(payload)-offset {
			return nil, fmt.Errorf("DATA_FRAME_ERROR: pad length out of range")
		}
	}
	// 提取数据（排除填充）
	dataLength := len(payload) - offset - int(frame.PadLen)
	if dataLength < 0 {
		return nil, fmt.Errorf("DATA_FRAME_ERROR: data length out of range: %d", dataLength)
	}
	frame.Data = make([]byte, dataLength)
	copy(frame.Data, payload[offset:offset+dataLength])
	return frame, nil
}

// =========================== SettingsFrame ===========================
// SettingFrame 序列化
func (f *SettingsFrame) Serialize() ([]byte, error) {
	size := len(f.Settings)
	f.Length = uint32(size * 6)
	header := f.FrameHeader.Serialize()

	// 欲分配完整帧
	offset := len(header)
	frame := make([]byte, offset+size*6)
	copy(frame, header)

	for _, setting := range f.Settings {
		binary.BigEndian.PutUint16(frame[offset:], setting.ID)
		binary.BigEndian.PutUint32(frame[offset+2:], setting.Value)
		offset += 6
	}
	return frame, nil
}

// SettingsFrame 解析
func ParseSettingsFrame(header *FrameHeader, payload []byte) (*SettingsFrame, error) {
	if header.StreamID != 0 {
		return nil, fmt.Errorf("SETTINGS_FRAME_ERROR: streamID must be zero")
	}

	if len(payload)%6 != 0 {
		return nil, fmt.Errorf("SETTINGS_FRAME_ERROR: invalid payload length")
	}
	settings := make([]Setting, len(payload)/6)
	for i := 0; i < len(settings); i++ {
		offset := i * 6
		settings[i] = Setting{
			ID:    binary.BigEndian.Uint16(payload[offset : offset+2]),
			Value: binary.BigEndian.Uint32(payload[offset+2 : offset+6]),
		}
	}
	return &SettingsFrame{
		FrameHeader: *header,
		Settings:    settings,
	}, nil
}

// 创建设置帧
func NewSettingsFrame(settings []Setting) *SettingsFrame {
	return &SettingsFrame{
		FrameHeader: FrameHeader{
			Length:   uint32(len(settings) * 6),
			Type:     FrameSettings,
			Flags:    0,
			StreamID: 0,
		},
		Settings: settings,
	}
}
