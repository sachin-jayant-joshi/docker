// +build windows

package term

import (
	"fmt"
	"io"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"unsafe"
)

const (
	// Consts for Get/SetConsoleMode function
	// see http://msdn.microsoft.com/en-us/library/windows/desktop/ms683167(v=vs.85).aspx
	ENABLE_ECHO_INPUT      = 0x0004
	ENABLE_INSERT_MODE     = 0x0020
	ENABLE_LINE_INPUT      = 0x0002
	ENABLE_MOUSE_INPUT     = 0x0010
	ENABLE_PROCESSED_INPUT = 0x0001
	ENABLE_QUICK_EDIT_MODE = 0x0040
	ENABLE_WINDOW_INPUT    = 0x0008
	// If parameter is a screen buffer handle, additional values
	ENABLE_PROCESSED_OUTPUT   = 0x0001
	ENABLE_WRAP_AT_EOL_OUTPUT = 0x0002

	//http://msdn.microsoft.com/en-us/library/windows/desktop/ms682088(v=vs.85).aspx#_win32_character_attributes
	FOREGROUND_BLUE       = 1
	FOREGROUND_GREEN      = 2
	FOREGROUND_RED        = 4
	FOREGROUND_INTENSITY  = 8
	FOREGROUND_MASK_SET   = 0x000F
	FOREGROUND_MASK_UNSET = 0xFFF0

	BACKGROUND_BLUE       = 16
	BACKGROUND_GREEN      = 32
	BACKGROUND_RED        = 64
	BACKGROUND_INTENSITY  = 128
	BACKGROUND_MASK_SET   = 0x00F0
	BACKGROUND_MASK_UNSET = 0xFF0F

	COMMON_LVB_REVERSE_VIDEO = 0x4000
	COMMON_LVB_UNDERSCORE    = 0x8000

	// http://man7.org/linux/man-pages/man4/console_codes.4.html
	// ECMA-48 Set Graphics Rendition
	ANSI_ATTR_RESET     = 0
	ANSI_ATTR_BOLD      = 1
	ANSI_ATTR_DIM       = 2
	ANSI_ATTR_UNDERLINE = 4
	ANSI_ATTR_BLINK     = 5
	ANSI_ATTR_REVERSE   = 7
	ANSI_ATTR_INVISIBLE = 8

	ANSI_ATTR_UNDERLINE_OFF = 24
	ANSI_ATTR_BLINK_OFF     = 25
	ANSI_ATTR_REVERSE_OFF   = 27
	ANSI_ATTR_INVISIBLE_OFF = 8

	ANSI_FOREGROUND_BLACK   = 30
	ANSI_FOREGROUND_RED     = 31
	ANSI_FOREGROUND_GREEN   = 32
	ANSI_FOREGROUND_YELLOW  = 33
	ANSI_FOREGROUND_BLUE    = 34
	ANSI_FOREGROUND_MAGENTA = 35
	ANSI_FOREGROUND_CYAN    = 36
	ANSI_FOREGROUND_WHITE   = 37
	ANSI_FOREGROUND_DEFAULT = 39

	ANSI_BACKGROUND_BLACK   = 40
	ANSI_BACKGROUND_RED     = 41
	ANSI_BACKGROUND_GREEN   = 42
	ANSI_BACKGROUND_YELLOW  = 43
	ANSI_BACKGROUND_BLUE    = 44
	ANSI_BACKGROUND_MAGENTA = 45
	ANSI_BACKGROUND_CYAN    = 46
	ANSI_BACKGROUND_WHITE   = 47
	ANSI_BACKGROUND_DEFAULT = 49

	ANSI_MAX_CMD_LENGTH = 256

	// https://msdn.microsoft.com/en-us/library/windows/desktop/ms683231(v=vs.85).aspx
	STD_INPUT_HANDLE  = -10
	STD_OUTPUT_HANDLE = -11
	STD_ERROR_HANDLE  = -12
)

// http://msdn.microsoft.com/en-us/library/windows/desktop/dd375731(v=vs.85).aspx
const (
	VK_PRIOR    = 0x21 // PAGE UP key
	VK_NEXT     = 0x22 // PAGE DOWN key
	VK_END      = 0x23 // END key
	VK_HOME     = 0x24 // HOME key
	VK_LEFT     = 0x25 // LEFT ARROW key
	VK_UP       = 0x26 // UP ARROW key
	VK_RIGHT    = 0x27 //RIGHT ARROW key
	VK_DOWN     = 0x28 //DOWN ARROW key
	VK_SELECT   = 0x29 //SELECT key
	VK_PRINT    = 0x2A //PRINT key
	VK_EXECUTE  = 0x2B //EXECUTE key
	VK_SNAPSHOT = 0x2C //PRINT SCREEN key
	VK_INSERT   = 0x2D //INS key
	VK_DELETE   = 0x2E //DEL key
	VK_HELP     = 0x2F //HELP key
	VK_F1       = 0x70 //F1 key
	VK_F2       = 0x71 //F2 key
	VK_F3       = 0x72 //F3 key
	VK_F4       = 0x73 //F4 key
	VK_F5       = 0x74 //F5 key
	VK_F6       = 0x75 //F6 key
	VK_F7       = 0x76 //F7 key
	VK_F8       = 0x77 //F8 key
	VK_F9       = 0x78 //F9 key
	VK_F10      = 0x79 //F10 key
	VK_F11      = 0x7A //F11 key
	VK_F12      = 0x7B //F12 key
)

var kernel32DLL = syscall.NewLazyDLL("kernel32.dll")

var (
	setConsoleModeProc                = kernel32DLL.NewProc("SetConsoleMode")
	getConsoleScreenBufferInfoProc    = kernel32DLL.NewProc("GetConsoleScreenBufferInfo")
	setConsoleCursorPositionProc      = kernel32DLL.NewProc("SetConsoleCursorPosition")
	setConsoleTextAttributeProc       = kernel32DLL.NewProc("SetConsoleTextAttribute")
	fillConsoleOutputCharacterProc    = kernel32DLL.NewProc("FillConsoleOutputCharacterW")
	readConsoleInputProc              = kernel32DLL.NewProc("ReadConsoleInputW")
	getNumberOfConsoleInputEventsProc = kernel32DLL.NewProc("GetNumberOfConsoleInputEvents")
	getConsoleCursorInfoProc          = kernel32DLL.NewProc("GetConsoleCursorInfo")
	setConsoleCursorInfoProc          = kernel32DLL.NewProc("SetConsoleCursorInfo")
)

// types for calling GetConsoleScreenBufferInfo
// see http://msdn.microsoft.com/en-us/library/windows/desktop/ms682093(v=vs.85).aspx
type (
	SHORT      int16
	SMALL_RECT struct {
		Left   SHORT
		Top    SHORT
		Right  SHORT
		Bottom SHORT
	}

	COORD struct {
		X SHORT
		Y SHORT
	}

	BOOL  int
	WORD  uint16
	WCHAR uint16
	DWORD uint32

	CONSOLE_SCREEN_BUFFER_INFO struct {
		Size              COORD
		CursorPosition    COORD
		Attributes        WORD
		Window            SMALL_RECT
		MaximumWindowSize COORD
	}

	CONSOLE_CURSOR_INFO struct {
		Size    DWORD
		Visible BOOL
	}

	// http://msdn.microsoft.com/en-us/library/windows/desktop/ms684166(v=vs.85).aspx
	KEY_EVENT_RECORD struct {
		KeyDown         BOOL
		RepeatCount     WORD
		VirtualKeyCode  WORD
		VirtualScanCode WORD
		UnicodeChar     WCHAR
		ControlKeyState DWORD
	}

	INPUT_RECORD struct {
		EventType WORD
		KeyEvent  KEY_EVENT_RECORD
	}
)

// Implements the TerminalEmulator interface
type WindowsTerminal struct {
	outMutex sync.Mutex
	inMutex  sync.Mutex
}

func NewTerminal(stdOut io.Writer, stdErr io.Writer, stdIn io.Reader) *Terminal {
	handler := &WindowsTerminal{}
	return &Terminal{
		StdOut: &terminalWriter{
			wrappedWriter: stdOut,
			emulator:      handler,
			command:       make([]byte, 0, ANSI_MAX_CMD_LENGTH),
		},
		StdErr: &terminalWriter{
			wrappedWriter: stdErr,
			emulator:      handler,
			command:       make([]byte, 0, ANSI_MAX_CMD_LENGTH),
		},
		StdIn: &terminalReader{
			wrappedReader: stdIn,
			emulator:      handler,
			command:       make([]byte, 0, ANSI_MAX_CMD_LENGTH),
		},
	}
}

//http://msdn.microsoft.com/en-us/library/windows/desktop/ms683167(v=vs.85).aspx
func GetConsoleMode(fileDesc uintptr) (uint32, error) {
	var mode uint32
	err := syscall.GetConsoleMode(syscall.Handle(fileDesc), &mode)
	return mode, err
}

//http://msdn.microsoft.com/en-us/library/windows/desktop/ms686033(v=vs.85).aspx
func SetConsoleMode(fileDesc uintptr, mode uint32) error {
	r, _, err := setConsoleModeProc.Call(fileDesc, uintptr(mode), 0)
	if r == 0 {
		if err != nil {
			return err
		}
		return syscall.EINVAL
	}
	return nil
}

// http://msdn.microsoft.com/en-us/library/windows/desktop/ms686019(v=vs.85).aspx
func SetCursorVisible(fileDesc uintptr, isVisible BOOL) (bool, error) {
	var cursorInfo CONSOLE_CURSOR_INFO
	r, _, err := getConsoleCursorInfoProc.Call(uintptr(fileDesc), uintptr(unsafe.Pointer(&cursorInfo)), 0)
	if r == 0 {
		if err != nil {
			return false, err
		}
		return false, syscall.EINVAL
	}
	cursorInfo.Visible = isVisible
	r, _, err = setConsoleCursorInfoProc.Call(uintptr(fileDesc), uintptr(unsafe.Pointer(&cursorInfo)), 0)
	if r == 0 {
		if err != nil {
			return false, err
		}
		return false, syscall.EINVAL
	}
	return true, nil
}

//http://msdn.microsoft.com/en-us/library/windows/desktop/ms683171(v=vs.85).aspx
func GetConsoleScreenBufferInfo(fileDesc uintptr) (*CONSOLE_SCREEN_BUFFER_INFO, error) {
	var info CONSOLE_SCREEN_BUFFER_INFO
	r, _, err := getConsoleScreenBufferInfoProc.Call(uintptr(fileDesc), uintptr(unsafe.Pointer(&info)), 0)
	if r == 0 {
		if err != nil {
			return nil, err
		}
		return nil, syscall.EINVAL
	}
	return &info, nil
}

// http://msdn.microsoft.com/en-us/library/windows/desktop/ms686047(v=vs.85).aspx
func setConsoleTextAttribute(fileDesc uintptr, attribute WORD) (bool, error) {
	r, _, err := setConsoleTextAttributeProc.Call(uintptr(fileDesc), uintptr(attribute), 0)
	if r == 0 {
		if err != nil {
			return false, err
		}
		return false, syscall.EINVAL
	}
	return true, nil
}

// http://msdn.microsoft.com/en-us/library/windows/desktop/ms682663(v=vs.85).aspx
func fillConsoleOutputCharacter(fileDesc uintptr, fillChar byte, length uint32, writeCord COORD) (bool, error) {
	out := int64(0)
	r, _, err := fillConsoleOutputCharacterProc.Call(uintptr(fileDesc), uintptr(fillChar), uintptr(length), uintptr(marshal(writeCord)), uintptr(unsafe.Pointer(&out)))
	// If the function succeeds, the return value is nonzero.
	if r == 0 {
		if err != nil {
			return false, err
		}
		return false, syscall.EINVAL
	}
	return true, nil
}

// Gets the number of space characters to write for "clearing" the section of terminal
func getNumberOfChars(fromCoord COORD, toCoord COORD, screenSize COORD) uint32 {
	// must be valid cursor position
	if fromCoord.X < 0 || fromCoord.Y < 0 || toCoord.X < 0 || toCoord.Y < 0 {
		return 0
	}
	if fromCoord.X >= screenSize.X || fromCoord.Y >= screenSize.Y || toCoord.X >= screenSize.X || toCoord.Y >= screenSize.Y {
		return 0
	}
	// can't be backwards
	if fromCoord.Y > toCoord.Y {
		return 0
	}
	// same line
	if fromCoord.Y == toCoord.Y {
		return uint32(toCoord.X-fromCoord.X) + 1
	}
	// spans more than one line
	if fromCoord.Y < toCoord.Y {
		// from start till end of line for first line +  from start of line till end
		retValue := uint32(screenSize.X-fromCoord.X) + uint32(toCoord.X) + 1
		// don't count first and last line
		linesBetween := toCoord.Y - fromCoord.Y - 1
		if linesBetween > 0 {
			retValue = retValue + uint32(linesBetween*screenSize.X)
		}
		return retValue
	}
	return 0
}

func clearDisplayRange(fileDesc uintptr, fillChar byte, fromCoord COORD, toCoord COORD, windowSize COORD) (bool, error) {
	totalChars := getNumberOfChars(fromCoord, toCoord, windowSize)
	if totalChars > 0 {
		r, err := fillConsoleOutputCharacter(fileDesc, fillChar, totalChars, fromCoord)

		if !r {
			if err != nil {
				return false, err
			}
			return false, syscall.EINVAL
		}
	}
	return true, nil
}

// setConsoleCursorPosition sets the console cursor position
// Note The X and Y are zero based
// If relative is true then the new position is relative to current one
func setConsoleCursorPosition(fileDesc uintptr, isRelative bool, column int16, line int16) (bool, error) {
	screenBufferInfo, err := GetConsoleScreenBufferInfo(fileDesc)
	if err == nil {
		var position COORD
		if isRelative {
			position.X = screenBufferInfo.CursorPosition.X + SHORT(column)
			position.Y = screenBufferInfo.CursorPosition.Y + SHORT(line)
		} else {
			position.X = SHORT(column)
			position.Y = SHORT(line)
		}

		//convert
		bits := marshal(position)
		r, _, err := setConsoleCursorPositionProc.Call(uintptr(fileDesc), uintptr(bits), 0)
		if r == 0 {
			if err != nil {
				return false, err
			}
			return false, syscall.EINVAL
		}
		return true, nil
	}
	return false, err
}

// http://msdn.microsoft.com/en-us/library/windows/desktop/ms683207(v=vs.85).aspx
func getNumberOfConsoleInputEvents(fileDesc uintptr) (uint16, error) {
	var n WORD
	r, _, err := getNumberOfConsoleInputEventsProc.Call(uintptr(fileDesc), uintptr(unsafe.Pointer(&n)))
	//If the function succeeds, the return value is nonzero
	if r != 0 {
		return uint16(n), nil
	}
	return 0, err
}

//http://msdn.microsoft.com/en-us/library/windows/desktop/ms684961(v=vs.85).aspx
func readConsoleInputKey(fileDesc uintptr, inputBuffer []INPUT_RECORD) (int, error) {
	var nr WORD
	r, _, err := readConsoleInputProc.Call(uintptr(fileDesc), uintptr(unsafe.Pointer(&inputBuffer[0])), uintptr(WORD(len(inputBuffer))), uintptr(unsafe.Pointer(&nr)))
	//If the function succeeds, the return value is nonzero.
	if r != 0 {
		return int(nr), nil
	}
	return int(0), err
}

func getWindowsTextAttributeForAnsiValue(originalFlag WORD, ansiValue int16) (WORD, error) {
	flag := WORD(originalFlag)
	if flag == 0 {
		// TODO - confirm this is the correct expectation
		flag = FOREGROUND_MASK_SET
	}
	switch ansiValue {
	case ANSI_ATTR_RESET:
		flag &^= COMMON_LVB_UNDERSCORE
		flag &^= BACKGROUND_INTENSITY
		flag = flag | FOREGROUND_INTENSITY
		// TODO: how do you reset reverse?
	case ANSI_ATTR_INVISIBLE:
		// TODO ??
	case ANSI_ATTR_UNDERLINE:
		flag = flag | COMMON_LVB_UNDERSCORE
	case ANSI_ATTR_BLINK:
		// seems like background intenisty is blink
		flag = flag | BACKGROUND_INTENSITY
	case ANSI_ATTR_UNDERLINE_OFF:
		flag &^= COMMON_LVB_UNDERSCORE
	case ANSI_ATTR_BLINK_OFF:
		// seems like background intenisty is blink
		flag &^= BACKGROUND_INTENSITY
	case ANSI_ATTR_BOLD:
		flag = flag | FOREGROUND_INTENSITY
	case ANSI_ATTR_DIM:
		flag &^= FOREGROUND_INTENSITY
	case ANSI_ATTR_REVERSE, ANSI_ATTR_REVERSE_OFF:
		// swap forground and background bits
		foreground := flag & FOREGROUND_MASK_SET
		background := flag & BACKGROUND_MASK_SET
		flag = (flag & BACKGROUND_MASK_UNSET & FOREGROUND_MASK_UNSET) | (foreground << 4) | (background >> 4)

	// FOREGROUND
	case ANSI_FOREGROUND_DEFAULT:
		flag = flag | FOREGROUND_MASK_SET
	case ANSI_FOREGROUND_BLACK:
		flag = flag ^ (FOREGROUND_RED | FOREGROUND_GREEN | FOREGROUND_BLUE)
	case ANSI_FOREGROUND_RED:
		flag = (flag & FOREGROUND_MASK_UNSET) | FOREGROUND_RED
	case ANSI_FOREGROUND_GREEN:
		flag = (flag & FOREGROUND_MASK_UNSET) | FOREGROUND_GREEN
	case ANSI_FOREGROUND_YELLOW:
		flag = (flag & FOREGROUND_MASK_UNSET) | FOREGROUND_RED | FOREGROUND_GREEN
	case ANSI_FOREGROUND_BLUE:
		flag = (flag & FOREGROUND_MASK_UNSET) | FOREGROUND_BLUE
	case ANSI_FOREGROUND_MAGENTA:
		flag = (flag & FOREGROUND_MASK_UNSET) | FOREGROUND_RED | FOREGROUND_BLUE
	case ANSI_FOREGROUND_CYAN:
		flag = (flag & FOREGROUND_MASK_UNSET) | FOREGROUND_GREEN | FOREGROUND_BLUE
	case ANSI_FOREGROUND_WHITE:
		flag = (flag & FOREGROUND_MASK_UNSET) | FOREGROUND_RED | FOREGROUND_GREEN | FOREGROUND_BLUE

	// Background
	case ANSI_BACKGROUND_DEFAULT:
		// Black with no intensity
		flag = (flag & BACKGROUND_MASK_UNSET)
	case ANSI_BACKGROUND_BLACK:
		flag = (flag & BACKGROUND_MASK_UNSET)
	case ANSI_BACKGROUND_RED:
		flag = (flag & BACKGROUND_MASK_UNSET) | BACKGROUND_RED
	case ANSI_BACKGROUND_GREEN:
		flag = (flag & BACKGROUND_MASK_UNSET) | BACKGROUND_GREEN
	case ANSI_BACKGROUND_YELLOW:
		flag = (flag & BACKGROUND_MASK_UNSET) | BACKGROUND_RED | BACKGROUND_GREEN
	case ANSI_BACKGROUND_BLUE:
		flag = (flag & BACKGROUND_MASK_UNSET) | BACKGROUND_BLUE
	case ANSI_BACKGROUND_MAGENTA:
		flag = (flag & BACKGROUND_MASK_UNSET) | BACKGROUND_RED | BACKGROUND_BLUE
	case ANSI_BACKGROUND_CYAN:
		flag = (flag & BACKGROUND_MASK_UNSET) | BACKGROUND_GREEN | BACKGROUND_BLUE
	case ANSI_BACKGROUND_WHITE:
		flag = (flag & BACKGROUND_MASK_UNSET) | BACKGROUND_RED | BACKGROUND_GREEN | BACKGROUND_BLUE
	default:
		// TODO: remove in final version
		panic("Should not reach here")

	}
	return flag, nil
}

// HandleOutputCommand interpretes the Ansi commands and then makes appropriate Win32 calls
func (term *WindowsTerminal) HandleOutputCommand(command []byte) (n int, err error) {
	// console settings changes need to happen in atomic way
	term.outMutex.Lock()
	defer term.outMutex.Unlock()

	r := false
	// Parse the command
	parsedCommand := parseAnsiCommand(command)

	// use appropriate handle
	handle, _ := syscall.GetStdHandle(STD_OUTPUT_HANDLE)

	switch parsedCommand.Command {
	case "m":
		// [Value;...;Valuem
		// Set Graphics Mode:
		// Calls the graphics functions specified by the following values.
		// These specified functions remain active until the next occurrence of this escape sequence.
		// Graphics mode changes the colors and attributes of text (such as bold and underline) displayed on the screen.
		flag := WORD(0)
		for _, e := range parsedCommand.Parameters {
			value, _ := strconv.ParseInt(e, 10, 16) // base 10, 16 bit
			flag, err = getWindowsTextAttributeForAnsiValue(flag, int16(value))
			if nil != err {
				return len(command), err
			}
		}

		r, err = setConsoleTextAttribute(uintptr(handle), flag)
		if !r {
			return len(command), err
		}
	case "H", "f":
		// [line;columnH
		// [line;columnf
		// Moves the cursor to the specified position (coordinates).
		// If you do not specify a position, the cursor moves to the home position at the upper-left corner of the screen (line 0, column 0).
		line, err := parseInt16OrDefault(parsedCommand.getParam(0), 1)
		if err != nil {
			return len(command), err
		}
		column, err := parseInt16OrDefault(parsedCommand.getParam(1), 1)
		if err != nil {
			return len(command), err
		}
		// The numbers are not 0 based, but 1 based
		r, err = setConsoleCursorPosition(uintptr(handle), false, int16(column-1), int16(line-1))
		if !r {
			return len(command), err
		}

	case "A":
		// [valueA
		// Moves the cursor up by the specified number of lines without changing columns.
		// If the cursor is already on the top line, ignores this sequence.
		value, err := parseInt16OrDefault(parsedCommand.getParam(0), 1)
		if err != nil {
			return len(command), err
		}
		r, err = setConsoleCursorPosition(uintptr(handle), true, 0, -1*value)
		if !r {
			return len(command), err
		}
	case "B":
		// [valueB
		// Moves the cursor down by the specified number of lines without changing columns.
		// If the cursor is already on the bottom line, ignores this sequence.
		value, err := parseInt16OrDefault(parsedCommand.getParam(0), 1)
		if err != nil {
			return len(command), err
		}
		r, err = setConsoleCursorPosition(uintptr(handle), true, 0, value)
		if !r {
			return len(command), err
		}
	case "C":
		// [valueC
		// Moves the cursor forward by the specified number of columns without changing lines.
		// If the cursor is already in the rightmost column, ignores this sequence.
		value, err := parseInt16OrDefault(parsedCommand.getParam(0), 1)
		if err != nil {
			return len(command), err
		}
		r, err = setConsoleCursorPosition(uintptr(handle), true, int16(value), 0)
		if !r {
			return len(command), err
		}
	case "D":
		// [valueD
		// Moves the cursor back by the specified number of columns without changing lines.
		// If the cursor is already in the leftmost column, ignores this sequence.
		value, err := parseInt16OrDefault(parsedCommand.getParam(0), 1)
		if err != nil {
			return len(command), err
		}
		r, err = setConsoleCursorPosition(uintptr(handle), true, int16(-1*value), 0)
		if !r {
			return len(command), err
		}
	case "J":
		// [J   Erases from the cursor to the end of the screen, including the cursor position.
		// [1J  Erases from the beginning of the screen to the cursor, including the cursor position.
		// [2J  Erases the complete display. The cursor does not move.
		// Clears the screen and moves the cursor to the home position (line 0, column 0).
		value, err := parseInt16OrDefault(parsedCommand.getParam(0), 0)
		if err != nil {
			return len(command), err
		}
		var start COORD
		var cursor COORD
		var end COORD
		screenBufferInfo, err := GetConsoleScreenBufferInfo(uintptr(handle))
		if err == nil {

			switch value {
			case 0:
				start = screenBufferInfo.CursorPosition
				// end of the screen
				end.X = screenBufferInfo.MaximumWindowSize.X - 1
				end.Y = screenBufferInfo.MaximumWindowSize.Y - 1
				// cursor
				cursor = screenBufferInfo.CursorPosition
			case 1:

				// start of the screen
				start.X = 0
				start.Y = 0
				// end of the screen
				end = screenBufferInfo.CursorPosition
				// cursor
				cursor = screenBufferInfo.CursorPosition
			case 2:
				// start of the screen
				start.X = 0
				start.Y = 0
				// end of the screen
				end.X = screenBufferInfo.MaximumWindowSize.X - 1
				end.Y = screenBufferInfo.MaximumWindowSize.Y - 1
				// cursor
				cursor.X = 0
				cursor.Y = 0
			}
			r, err = clearDisplayRange(uintptr(handle), ' ', start, end, screenBufferInfo.MaximumWindowSize)
			if !r {
				return len(command), err
			}
			// remember the the cursor position is 1 based
			r, err = setConsoleCursorPosition(uintptr(handle), false, int16(cursor.X), int16(cursor.Y))
			if !r {
				return len(command), err
			}
		}
	case "K":
		// [K
		// Clears all characters from the cursor position to the end of the line (including the character at the cursor position).
		// [K  Erases from the cursor to the end of the line, including the cursor position.
		// [1K  Erases from the beginning of the line to the cursor, including the cursor position.
		// [2K  Erases the complete line.
		value, err := parseInt16OrDefault(parsedCommand.getParam(0), 0)
		var start COORD
		var cursor COORD
		var end COORD
		screenBufferInfo, err := GetConsoleScreenBufferInfo(uintptr(handle))
		if err == nil {

			switch value {
			case 0:
				// start is where cursor is
				start = screenBufferInfo.CursorPosition
				// end of line
				end.X = screenBufferInfo.MaximumWindowSize.X - 1
				end.Y = screenBufferInfo.CursorPosition.Y
				// cursor remains the same
				cursor = screenBufferInfo.CursorPosition

			case 1:
				// beginning of line
				start.X = 0
				start.Y = screenBufferInfo.CursorPosition.Y
				// until cursor
				end = screenBufferInfo.CursorPosition
				// cursor remains the same
				cursor = screenBufferInfo.CursorPosition
			case 2:
				// start of the line
				start.X = 0
				start.Y = screenBufferInfo.MaximumWindowSize.Y - 1
				// end of the line
				end.X = screenBufferInfo.MaximumWindowSize.X - 1
				end.Y = screenBufferInfo.MaximumWindowSize.Y - 1
				// cursor
				cursor.X = 0
				cursor.Y = screenBufferInfo.MaximumWindowSize.Y - 1
			}
			r, err = clearDisplayRange(uintptr(handle), ' ', start, end, screenBufferInfo.MaximumWindowSize)
			if !r {
				return len(command), err
			}
			// remember the the cursor position is 1 based
			r, err = setConsoleCursorPosition(uintptr(handle), false, int16(cursor.X), int16(cursor.Y))
			if !r {
				return len(command), err
			}
		}

	case "l":
		value := parsedCommand.getParam(0)
		if value == "?25" {
			SetCursorVisible(uintptr(handle), BOOL(0))
		}
	case "h":
		value := parsedCommand.getParam(0)
		if value == "?25" {
			SetCursorVisible(uintptr(handle), BOOL(1))
		}

	case "]":
	/*
		TODO (azlinux):
			Linux Console Private CSI Sequences

		       The following sequences are neither ECMA-48 nor native VT102.  They are
		       native  to the Linux console driver.  Colors are in SGR parameters: 0 =
		       black, 1 = red, 2 = green, 3 = brown, 4 = blue, 5 = magenta, 6 =  cyan,
		       7 = white.

		       ESC [ 1 ; n ]       Set color n as the underline color
		       ESC [ 2 ; n ]       Set color n as the dim color
		       ESC [ 8 ]           Make the current color pair the default attributes.
		       ESC [ 9 ; n ]       Set screen blank timeout to n minutes.
		       ESC [ 10 ; n ]      Set bell frequency in Hz.
		       ESC [ 11 ; n ]      Set bell duration in msec.
		       ESC [ 12 ; n ]      Bring specified console to the front.
		       ESC [ 13 ]          Unblank the screen.
		       ESC [ 14 ; n ]      Set the VESA powerdown interval in minutes.

	*/
	default:
		//if !parsedCommand.IsSpecial {
		//fmt.Printf("%+v %+v\n", string(command), parsedCommand)
		//}
	}
	return len(command), nil
}

func (term *WindowsTerminal) WriteChars(w io.Writer, p []byte) (n int, err error) {
	return w.Write(p)
}

const (
	CAPSLOCK_ON        = 0x0080 //The CAPS LOCK light is on.
	ENHANCED_KEY       = 0x0100 //The key is enhanced.
	LEFT_ALT_PRESSED   = 0x0002 //The left ALT key is pressed.
	LEFT_CTRL_PRESSED  = 0x0008 //The left CTRL key is pressed.
	NUMLOCK_ON         = 0x0020 //The NUM LOCK light is on.
	RIGHT_ALT_PRESSED  = 0x0001 //The right ALT key is pressed.
	RIGHT_CTRL_PRESSED = 0x0004 //The right CTRL key is pressed.
	SCROLLLOCK_ON      = 0x0040 //The SCROLL LOCK light is on.
	SHIFT_PRESSED      = 0x0010 // The SHIFT key is pressed.
)

const (
	KEY_CONTROL_PARAM_2 = ";2"
	KEY_CONTROL_PARAM_3 = ";3"
	KEY_CONTROL_PARAM_4 = ";4"
	KEY_CONTROL_PARAM_5 = ";5"
	KEY_CONTROL_PARAM_6 = ";6"
	KEY_CONTROL_PARAM_7 = ";7"
	KEY_CONTROL_PARAM_8 = ";8"
	KEY_ESC_N           = "\x1BN"
)

var keyMapPrefix = map[WORD]string{
	VK_UP:     "\x1B[%sA",
	VK_DOWN:   "\x1B[%sB",
	VK_RIGHT:  "\x1B[%sC",
	VK_LEFT:   "\x1B[%sD",
	VK_HOME:   "\x1B[H%s~",
	VK_END:    "\x1B[F%s~",
	VK_INSERT: "\x1B[2%s~",
	VK_DELETE: "\x1B[3%s~",
	VK_F1:     "",
	VK_F2:     "",
	VK_F3:     "\x1B[13%s~",
	VK_F4:     "\x1B[14%s~",
	VK_F5:     "\x1B[15%s~",
	VK_F6:     "\x1B[17%s~",
	VK_F7:     "\x1B[18%s~",
	VK_F8:     "\x1B[19%s~",
	VK_F9:     "\x1B[20%s~",
	VK_F10:    "\x1B[21%s~",
	VK_F11:    "\x1B[23%s~",
	VK_F12:    "\x1B[24%s~",
}

func getControlStateParameter(shift, alt, control, meta bool) string {
	if shift && alt && control {
		return KEY_CONTROL_PARAM_8
	}
	if alt && control {
		return KEY_CONTROL_PARAM_7
	}
	if shift && control {
		return KEY_CONTROL_PARAM_6
	}
	if control {
		return KEY_CONTROL_PARAM_5
	}
	if shift && alt {
		return KEY_CONTROL_PARAM_4
	}
	if alt {
		return KEY_CONTROL_PARAM_3
	}
	if shift {
		return KEY_CONTROL_PARAM_2
	}
	return ""
}

func getControlKeys(controlState DWORD) (shift, alt, control bool) {
	shift = 0 != (controlState & SHIFT_PRESSED)
	alt = 0 != (controlState & (LEFT_ALT_PRESSED | RIGHT_ALT_PRESSED))
	control = 0 != (controlState & (LEFT_CTRL_PRESSED | RIGHT_CTRL_PRESSED))
	return shift, alt, control
}

func charSequenceForKeys(key WORD, controlState DWORD) string {
	i, ok := keyMapPrefix[key]
	if ok {
		shift, alt, control := getControlKeys(controlState)
		modifier := getControlStateParameter(shift, alt, control, false)
		return fmt.Sprintf(i, modifier)
	} else {
		return ""
	}
}

func mapKeystokeToTerminalString(keyEvent *KEY_EVENT_RECORD) string {
	_, alt, control := getControlKeys(keyEvent.ControlKeyState)
	if keyEvent.UnicodeChar == 0 {
		return charSequenceForKeys(keyEvent.VirtualKeyCode, keyEvent.ControlKeyState)
	}
	if control {
		// TODO(azlinux):
		// <Ctrl>-D  Signals the end of input from the keyboard; also exits current shell.
		// <Ctrl>-H  Deletes the first character to the left of the cursor. Also called the ERASE key.
		// <Ctrl>-Q  Restarts printing after it has been stopped with <Ctrl>-s.
		// <Ctrl>-S  Suspends printing on the screen (does not stop the program).
		// <Ctrl>-U  Deletes all characters on the current line. Also called the KILL key.
		// <Ctrl>-E  Quits current command and creates a core

	}
	// <Alt>+Key generates ESC N Key
	if !control && alt {
		return KEY_ESC_N + strings.ToLower(string(keyEvent.UnicodeChar))
	}
	return string(keyEvent.UnicodeChar)
}

func (term *WindowsTerminal) ReadChars(w io.Reader, p []byte) (n int, err error) {
	handle, _ := syscall.GetStdHandle(STD_INPUT_HANDLE)
	if nil != err {
		return 0, err
	}
	// Read number of console events available
	nEvents, err := getNumberOfConsoleInputEvents(uintptr(handle))
	if nil != err {
		return 0, err
	}
	if 0 == nEvents {
		return 0, nil
	}
	// Read the keystrokes
	inputBuffer := make([]INPUT_RECORD, int(nEvents)+1)
	nr, err := readConsoleInputKey(uintptr(handle), inputBuffer)
	if nil != err {
		return 0, err
	}
	if 0 == nr {
		return 0, nil
	}
	// Process the keystrokes
	charIndex := 0
	for i := 0; i < nr; i++ {
		input := inputBuffer[i]
		if input.EventType == KEY_EVENT && input.KeyEvent.KeyDown == 1 {
			keyString := mapKeystokeToTerminalString(&input.KeyEvent)
			if len(keyString) > 0 {
				for _, e := range keyString {
					p[charIndex] = byte(e)
					charIndex++
				}
			}
		}
		if charIndex >= len(p) && charIndex > 0 {
			break
		}
	}
	return charIndex, nil
}

func (term *WindowsTerminal) HandleInputSequence(command []byte) (n int, err error) {
	term.inMutex.Lock()
	defer term.inMutex.Unlock()
	return 0, nil
}

// TODO: once the code is working rock solid remove all asserts
func assert(cond bool, format string, a ...interface{}) {
	if !cond {
		panic(fmt.Sprintf(format, a))
	}
}

func marshal(c COORD) uint32 {
	// works only on intel-endian machines
	// TODO(azlinux): make it so that it does not fail
	return uint32(uint32(uint16(c.Y))<<16 | uint32(uint16(c.X)))
}
