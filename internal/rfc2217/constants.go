package rfc2217

// Telnet protocol constants
const (
	IAC  byte = 255 // Interpret As Command
	DONT byte = 254
	DO   byte = 253
	WONT byte = 252
	WILL byte = 251
	SB   byte = 250 // Subnegotiation Begin
	GA   byte = 249 // Go Ahead
	EL   byte = 248 // Erase Line
	EC   byte = 247 // Erase Character
	AYT  byte = 246 // Are You There
	AO   byte = 245 // Abort Output
	IP   byte = 244 // Interrupt Process
	BRK  byte = 243 // Break
	DM   byte = 242 // Data Mark
	NOP  byte = 241 // No Operation
	SE   byte = 240 // Subnegotiation End
)

// RFC-2217 COM Port Control option
const (
	ComPortOption byte = 44 // COM-PORT-OPTION
)

// RFC-2217 Subnegotiation commands (client to server)
const (
	SignatureC        byte = 0
	SetBaudrateC      byte = 1
	SetDatasizeC      byte = 2
	SetParityC        byte = 3
	SetStopSizeC      byte = 4
	SetControlC       byte = 5
	NotifyLinestateC  byte = 6
	NotifyModemstateC byte = 7
	FlowControlSuspC  byte = 8
	FlowControlResC   byte = 9
	SetLinestateC     byte = 10
	SetModemstateC    byte = 11
	PurgeDataC        byte = 12
)

// RFC-2217 Subnegotiation commands (server to client, +100)
const (
	SignatureS        byte = 100
	SetBaudrateS      byte = 101
	SetDatasizeS      byte = 102
	SetParityS        byte = 103
	SetStopSizeS      byte = 104
	SetControlS       byte = 105
	NotifyLinestateS  byte = 106
	NotifyModemstateS byte = 107
	FlowControlSuspS  byte = 108
	FlowControlResS   byte = 109
	SetLinestateS     byte = 110
	SetModemstateS    byte = 111
	PurgeDataS        byte = 112
)

// Parity values
const (
	ParityNone  byte = 1
	ParityOdd   byte = 2
	ParityEven  byte = 3
	ParityMark  byte = 4
	ParitySpace byte = 5
)

// Stop bits values
const (
	StopBits1   byte = 1
	StopBits2   byte = 2
	StopBits1_5 byte = 3
)

// Flow control values
const (
	FlowControlNone    byte = 1
	FlowControlXonXoff byte = 2
	FlowControlRtsCts  byte = 3
)
