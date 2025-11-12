package http2

// HTTP/2 常量定义
const (
	Preface = "PRI * HTTP/2.0\r\n\r\nSM\r\n\r\n"

	// 帧类型
	FrameData         = 0x0 // 数据帧 - 传输请求/响应体
	FrameHeaders      = 0x1 // 头部帧 - 传输 HTTP 头部
	FramePriority     = 0x2 // 优先级帧 - 设置流优先级
	FrameRSTStream    = 0x3 // 流终止帧 - 立即终止流
	FrameSettings     = 0x4 // 设置帧 - 连接参数协商
	FramePushPromise  = 0x5 // 推送承诺帧 - 服务器推送资源
	FramePing         = 0x6 // Ping 帧 - 连接保活和延迟测量
	FrameGoWay        = 0x7 // GoAway 帧 - 优雅关闭连接
	FrameWindowUpdate = 0x8 // 窗口更新帧 - 流量控制
	FrameContinuation = 0x9 // 延续帧 - 继续传输头部块

	// 标志位（历史原因导致不是连续的2点幂次方）
	FlagEndStream  = FlagBit0 // 0x1
	FlagEndHeaders = FlagBit2 // 0x4
	FlagPadded     = FlagBit3 // 0x8
	FlagPriority   = FlagBit5 // 0x20

	// 初始化窗口大小
	InitialWindowSize = 65535 // 64 KB

	// 最大帧大小
	MaxFrameSize = 16384 // 16 KB

	FlagBit0 = 1 << 0 // 0x01 0000 0001
	FlagBit1 = 1 << 1 // 0x02 0000 0010
	FlagBit2 = 1 << 2 // 0x04 0000 0100
	FlagBit3 = 1 << 3 // 0x08 0000 1000
	FlagBit4 = 1 << 4 // 0x10 0001 0000
	FlagBit5 = 1 << 5 // 0x20 0010 0000
	FlagBit6 = 1 << 6 // 0x40 0100 0000
	FlagBit7 = 1 << 7 // 0x80 1000 0000
)
