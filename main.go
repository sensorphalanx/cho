package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"os/signal"
	"runtime"
	"strconv"
	"strings"
	"unicode"

	"github.com/mattn/go-colorable"
	"github.com/mattn/go-runewidth"
	"github.com/mattn/go-tty"
)

const name = "cho"
const version = "0.0.6"

var revision = "HEAD"

type AnsiColor map[string]string

func (a AnsiColor) Get(name, fallback string) string {
	if c, ok := a[name]; ok {
		return c
	}
	return a[fallback]
}

var (
	cursorline  = flag.Bool("cl", false, "Cursor line")
	linefg      = flag.String("lf", "black", "Line foreground")
	linebg      = flag.String("lb", "white", "Line background")
	color       = flag.Bool("cc", false, "Handle colors")
	query       = flag.Bool("q", false, "Use query")
	multi       = flag.Bool("m", false, "Multi select")
	maxlines    = flag.Int("M", -1, "Max lines")
	sep         = flag.String("sep", "", "Separator for prefix")
	showVersion = flag.Bool("v", false, "Print the version")
	truncate    = runewidth.Truncate

	fgcolor = AnsiColor{
		"gray":    "30",
		"black":   "30",
		"red":     "31",
		"green":   "32",
		"yellow":  "33",
		"blue":    "34",
		"magenta": "35",
		"cyan":    "36",
		"white":   "37",
	}
	bgcolor = AnsiColor{
		"black":   "40",
		"gray":    "40",
		"red":     "41",
		"green":   "42",
		"yellow":  "43",
		"blue":    "44",
		"magenta": "45",
		"cyan":    "46",
		"white":   "47",
	}
)

func truncateAnsi(line string, w int, _ string) string {
	r := []rune(line)
	out := []rune{}
	width := 0
	i := 0
	for ; i < len(r); i++ {
		if i < len(r)-1 && r[i] == '\x1b' && r[i+1] == '[' {
			j := i + 2
			for ; j < len(r); j++ {
				if ('a' <= r[j] && r[j] <= 'z') || ('A' <= r[j] && r[j] <= 'Z') {
					if r[j] == 'm' {
						s := ""
						for _, tok := range strings.Split(string(r[i+2:j]), ";") {
							n, _ := strconv.Atoi(tok)
							if n == 0 || n == 39 || (30 <= n && n <= 37) {
								if s != "" {
									s += ";"
								}
								if n == 0 {
									tok = "39"
								}
								s += tok
							}
						}
						s = "\x1b[" + s + "m"
						out = append(out, []rune(s)...)
					}
					break
				}
			}
			i = j
			continue
		}
		cw := runewidth.RuneWidth(r[i])
		if width+cw > w {
			break
		}
		width += cw
		out = append(out, r[i])
	}
	return string(out)
}

func main() {
	flag.Parse()
	if *showVersion {
		fmt.Printf("%s %s (rev: %s/%s)\n", name, version, revision, runtime.Version())
		return
	}

	fillstart := "\x1b[0K"
	fillend := "\x1b[0m"
	clearend := "\x1b[0K"
	if *cursorline {
		fillstart = ""
		fillend = "\x1b[0K\x1b[0m"
	}
	fg := fgcolor.Get(*linefg, "black")
	bg := bgcolor.Get(*linebg, "white")

	if *color {
		truncate = truncateAnsi
	}

	b, err := ioutil.ReadAll(os.Stdin)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if len(b) == 0 {
		fmt.Fprintln(os.Stderr, "no buffer to work with was available")
		os.Exit(1)
	}
	lines := strings.Split(strings.Replace(strings.TrimSpace(string(b)), "\r", "", -1), "\n")
	result := ""
	var qlines, rlines []string

	tty, err := tty.Open()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	defer colorable.EnableColorsStdout(nil)()
	out := colorable.NewColorable(tty.Output())

	sc := make(chan os.Signal, 1)
	signal.Notify(sc, os.Interrupt)
	go func() {
		<-sc
		out.Write([]byte("\x1b[?25h\x1b[0J"))
		tty.Close()
		os.Exit(1)
	}()

	if *sep == "TAB" {
		*sep = "\t"
	}
	if !*query {
		out.Write([]byte("\x1b[?25l"))
	}

	defer func() {
		e := recover()
		out.Write([]byte("\x1b[?25h\r\x1b[0J"))
		tty.Close()
		if e != nil {
			panic(e)
		}
		if result != "" {
			fmt.Print(result)
		} else {
			os.Exit(1)
		}
	}()

	var rs []rune
	off := 0
	row := 0
	dirty := make([]bool, len(lines))
	selected := make([]bool, len(lines))
	for i := 0; i < len(dirty); i++ {
		dirty[i] = true
	}
	ml := 0
	mh := 0
	for {
		w, h, err := tty.Size()
		if err != nil {
			w = 79
			h = 25
		}
		if *multi {
			w -= 2
		}
		if *maxlines > 0 && *maxlines < h-1 {
			ml = *maxlines
		} else {
			ml = h - 2
		}
		if *query {
			mh = ml
		} else {
			mh = ml + 1
		}

		n := 0
		if *query {
			n++
			out.Write([]byte(fillstart))
			out.Write([]byte("\r" + clearend + "> " + string(rs) + "\n"))
			qlines = nil
			rlines = nil
			if len(rs) > 0 {
				for i, qline := range lines {
					rline := qline
					if *sep != "" {
						tok := strings.SplitN(qline, *sep, 2)
						if len(tok) == 2 {
							rline = tok[0]
							qline = tok[1]
						} else {
							rline = tok[0]
							qline = ""
						}
					}

					if strings.Index(qline, string(rs)) != -1 {
						rlines = append(rlines, rline)
						qlines = append(qlines, qline)
					}
					dirty[off+i] = true
				}
				out.Write([]byte("\x1b[0J"))
			} else {
				for _, qline := range lines {
					rline := qline
					if *sep != "" {
						tok := strings.SplitN(qline, *sep, 2)
						if len(tok) == 2 {
							rline = tok[0]
							qline = tok[1]
						} else {
							rline = tok[0]
							qline = ""
						}
					}

					rlines = append(rlines, rline)
					qlines = append(qlines, qline)
				}
			}
			if off >= len(qlines) {
				off = 0
			}
			qlines = qlines[off:]
			rlines = qlines
		} else {
			qlines = lines[off:]
			rlines = qlines
		}
		out.Write([]byte("\x1b[?25l"))

		for i, line := range qlines {
			line = strings.Replace(line, "\t", "    ", -1)
			line = truncate(line, w, "")
			if dirty[off+i] {
				if *multi {
					if selected[off+i] {
						out.Write([]byte{'*'})
					} else {
						out.Write([]byte{' '})
					}
				}
				out.Write([]byte(fillstart))
				if off+i == row {
					out.Write([]byte("\x1b[" + fg + ";" + bg + "m" + line + fillend + "\r"))
				} else {
					out.Write([]byte(line + clearend + "\r"))
				}
				dirty[off+i] = false
			}
			if n >= ml {
				break
			}
			out.Write([]byte("\n"))
			n++
		}
		if *query {
			out.Write([]byte("\x1b[?25h"))
		}
		if n >= 1 {
			out.Write([]byte(fmt.Sprintf("\x1b[%dA", n)))
		}
		if *query {
			out.Write([]byte(fmt.Sprintf("\x1b[%dC", runewidth.StringWidth(string(rs))+2)))
		}

		var r rune
		for {
			r, err = tty.ReadRune()
			if err != nil {
				panic(err)
			}
			if r != 0 {
				break
			}
		}

	retry:
		switch r {
		case 0x09, 0x0E: // TAB/CTRL-N
			if row < len(qlines)-1 {
				dirty[row], dirty[row+1] = true, true
				row++
				if row-off >= mh {
					off++
					for i := 0; i < len(dirty); i++ {
						dirty[i] = true
					}
				}
			}
		case 0x10: // CTRL-P
			if row > 0 {
				dirty[row], dirty[row-1] = true, true
				row--
				if row < off {
					off--
					for i := 0; i < len(dirty); i++ {
						dirty[i] = true
					}
				}
			}
		case 0x15, 0x17: // CTRL-U/CTRL-W
			if *query && len(rs) > 0 {
				rs = []rune{}
				row = 0
				off = 0
				for i := 0; i < len(dirty); i++ {
					dirty[i] = true
				}
			}
		case 0x16: // CTRL-V
			selected[row] = !selected[row]
			dirty[row] = true
		case 0x0D: // ENTER
			if *multi {
				for i, s := range selected {
					if s {
						if *sep != "" {
							result += rlines[i-off] + "\n"
						} else {
							result += qlines[i-off] + "\n"
						}
					}
				}
			} else {
				if *sep != "" {
					result = rlines[row-off] + "\n"
				} else {
					result = qlines[row-off] + "\n"
				}
			}
			return
		case 0x1B: // ESC
			if !tty.Buffered() {
				return
			}
			r, err = tty.ReadRune()
			if err == nil && r == 0x5b {
				r, err = tty.ReadRune()
				if err != nil {
					panic(err)
				}
				switch r {
				case 'A':
					r = 0x0E // ALLOW-UP
					goto retry
				case 'B': // ALLOW-DOWN
					r = 0x10
					goto retry
				case 'Z': // SHIFT-TAB
					r = 0x10
					goto retry
				}
			}
		case 0x08, 0x7F: // BS/DELETE
			if *query && len(rs) > 0 {
				rs = rs[:len(rs)-1]
				row = 0
				off = 0
				if len(rs) == 0 {
					for i := 0; i < len(dirty); i++ {
						dirty[i] = true
					}
				}
			}
		default:
			if !*query {
				switch r {
				case 'j':
					r = 0x0E
					goto retry
				case 'k':
					r = 0x10
					goto retry
				}
			}
			if *query && unicode.IsPrint(r) {
				rs = append(rs, r)
				row = 0
				off = 0
			}
		}
	}
}
