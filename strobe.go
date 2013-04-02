package main

import (
	"fmt"
	"github.com/aqua/raspberrypi/gpio"
	"strconv"
	"time"
	"os/exec"
	"bufio"
	"os"
	"io"
)

func strobe(g *gpio.GPIOLine, timing <-chan bool) {
	defer g.Close()
	for cur := range timing {
		g.SetState(cur)
	}
}

func openStrobe(line uint) chan<- bool {
	g, err := gpio.NewGPIOLine(line, gpio.OUT)
	if err != nil {
		// I'm lazy
		panic(err)
	}
	ret := make(chan bool, 5)
	go strobe(g, ret)
	return ret
}

const (
	FASTER = 100 + iota
	SLOWER
)

func getInput(out chan<- int) {
	for {
		var s string
		fmt.Scanf("%s", &s)
		fmt.Println("Got", s)
		if i, err := strconv.Atoi(s); err == nil {
			if i == -1 {
				close(out)
				return
			}
			out <- i
		} else {
			if s == ">" {
				out <- FASTER
			} else if s == "<" {
				out <- SLOWER
			}
		}
	}
}

type media struct {
	cmd *exec.Cmd
	stdin io.WriteCloser
	beatSource beat
}
type beatChange struct{
	time time.Duration
	period float32
}
type beat struct {
	changes []beatChange
}

func readBeat(path string) beat {
	fin, err := os.Open(path)
	if err != nil {
		panic(err)
	}
	lines := bufio.NewReader(fin)
	lastTime := float32(0)
	changes := make([]beatChange, 0, 30)
	for {
		if line, _, err := lines.ReadLine(); err != nil {
			break
		}else{
			if fl, err := strconv.ParseFloat(string(line), 32); err != nil {
				panic(err)
			} else {
				fl := float32(fl)
				period := fl - lastTime
				absTime := time.Duration(float32(time.Second) * fl)
				changes = append(changes, beatChange{absTime, period})
				lastTime = fl
			}
		}
	}
	fin.Close()
	return beat{changes}
}

func startMedia(path string) media {
	cmd := exec.Command("vlc-wrapper", "-I", "rc", "--play-and-stop", path)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		panic(err)
	}
	if err := cmd.Start(); err != nil {
		panic(err)
	}
	beat := readBeat(path + ".beat")
	if _, err := stdin.Write(([]byte)("pause\n")); err != nil {
		panic(err)
	}

	return media{cmd,stdin, beat}
}

func createTimeChan(period float32) *time.Ticker {
	return time.NewTicker(time.Duration(float32(time.Second)*(period/4.7)))
}

func (m media) Start() {
	t := m.stdin
	t.Write(([]byte)("play\n"))
	fmt.Println("Started vlc")
}

func detectBPM(m media, out chan<-*time.Ticker) {
	m.Start()
	start := time.Now()

	lastPeriod := m.beatSource.changes[0].period
	for _, ch := range m.beatSource.changes {
		for time.Now().Sub(start) < ch.time {
			time.Sleep(time.Millisecond*10)
		}
		thisPeriod := ch.period*0.5 + lastPeriod*0.5
		out<-createTimeChan(thisPeriod)
		lastPeriod = thisPeriod
	}
	fmt.Println("Out of beat changes!")
}

func main() {
	fmt.Println("Started!")
	chs := [...]struct {
		ch chan<- bool
		on bool
	}{
		{openStrobe(7), true},
		{openStrobe(8), true},
		{openStrobe(11), true},
	}

	vlc := startMedia("music/say.wav")
	time.Sleep(time.Second*1)

	bpmChange := make(chan *time.Ticker)
	go detectBPM(vlc, bpmChange)

	cmd := make(chan int)
	go getInput(cmd)
	cur := true
	var ticker *time.Ticker
	var tickChan <-chan time.Time
	for {
		select {
			case <- tickChan:
				for _, ch := range chs {
					if ch.on {
						ch.ch <- cur
					}
				}
				cur = !cur
			case newTicker := <-bpmChange:
				if ticker != nil {
					ticker.Stop()
				}
				ticker = newTicker
				tickChan = ticker.C
			case char := <-cmd:
				switch {
					case char >= 0 && char < len(chs):
						chs[char].on = !chs[char].on
						chs[char].ch <- false
					default:
						fmt.Println("Bad command:", char)
				}
		}

	}

}
