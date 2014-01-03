package main

import (
	"flag"
	"fmt"
	"log"
	"math/rand"
	"os"
	"sync"
	"time"

	"github.com/rakyll/portmidi"
)

var (
	deviceN = flag.Int("device", -1, "MIDI Device ID")
)

func main() {
	flag.Parse()
	rand.Seed(time.Now().Unix())

	if err := portmidi.Initialize(); err != nil {
		log.Fatal(err)
	}
	defer portmidi.Terminate()
	if *deviceN == -1 {
		listDevices()
		return
	}
	if err := run(); err != nil {
		portmidi.Terminate()
		log.Fatal(err)
	}
}

func listDevices() {
	n := portmidi.CountDevices()
	for i := 0; i < n; i++ {
		info := portmidi.GetDeviceInfo(portmidi.DeviceId(i))
		fmt.Printf("Device %d: %v\n", i, info)
	}
}

func run() error {
	s, err := portmidi.NewOutputStream(portmidi.DeviceId(*deviceN), 1024, 0)
	if err != nil {
		return err
	}
	defer s.Close()
	ls := &lockedStream{s: s}

	quit := make(chan bool)

	spd := 1000 * time.Millisecond

	n1 := make(chan Note)
	g1 := generator{
		minN: 48, maxN: 72,
		minD:    spd,
		maxD:    4 * spd,
		stepD:   spd / 8,
		scale:   other,
		ringLen: 16,
		noteP:   4,
		timeP:   2,
		switchP: 16,
	}
	go g1.run(n1, quit)
	go play(ls, 2, n1, quit)

	n2 := make(chan Note)
	g2 := generator{
		minN: 36, maxN: 60,
		minD:    4 * spd,
		maxD:    16 * spd,
		stepD:   spd,
		scale:   other,
		ringLen: 4,
		noteP:   2,
		timeP:   2,
		switchP: 4,
	}
	go g2.run(n2, quit)
	go play(ls, 3, n2, quit)

	b := make([]byte, 1)
	os.Stdin.Read(b)
	close(quit)

	time.Sleep(time.Second)
	return nil
}

type lockedStream struct {
	mu sync.Mutex
	s  *portmidi.Stream
}

func (s *lockedStream) WriteShort(status, data1, data2 int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.s.WriteShort(status, data1, data2)
}

func play(s *lockedStream, ch int, notes chan Note, quit chan bool) {
	for {
		select {
		case <-quit:
			return
		case n, ok := <-notes:
			if !ok {
				return
			}
			if n.n > 0 {
				s.WriteShort(0x90+int64(ch), n.n, 100)
			}
			q := false
			select {
			case <-quit:
				q = true
			case <-time.After(n.d):
			}
			if n.n > 0 {
				s.WriteShort(0x80+int64(ch), n.n, 100)
			}
			if q {
				return
			}
		}
	}
}

type Note struct {
	n int64
	d time.Duration
}

type generator struct {
	minN, maxN            int64
	minD, maxD, stepD     time.Duration
	scale                 Scale
	ringLen               int
	noteP, timeP, switchP int
}

func (g *generator) run(notes chan Note, quit chan bool) {
	ring := make([]Note, g.ringLen)
	for i := range ring {
		if i%2 == 0 {
			n := Note{int64(rand.Intn(int(g.maxN-g.minN))) + g.minN, g.minD}
			ring[i] = g.scale.Quantize(n)
		} else {
			ring[i] = Note{0, g.minD}
		}
	}
	for {
		for i := range ring {
			ring[i] = g.mutate(ring[i])
			notes <- ring[i]
		}
	}
}

func (g *generator) mutate(n Note) Note {
	if n.n > 0 && rand.Intn(g.noteP) == 0 {
		n.n += int64(rand.Intn(13) - 6)
	}
	if rand.Intn(g.switchP) == 0 {
		if n.n == 0 {
			n.n = int64(rand.Intn(int(g.maxN-g.minN))) + g.minN
		} else {
			n.n = 0
		}
	}
	if n.n > 0 {
		if n.n < g.minN {
			n.n = g.minN
		}
		if n.n > g.maxN {
			n.n = g.maxN
		}
		n = g.scale.Quantize(n)
	}
	if rand.Intn(g.timeP) == 0 {
		n.d += time.Duration(rand.Intn(3)-1) * g.stepD
		if n.d < g.minD {
			n.d = g.minD
		}
		if n.d > g.maxD {
			n.d = g.maxD
		}
	}
	return n
}

type Scale [12]bool

func (s Scale) Quantize(note Note) Note {
	oct := note.n / 12
	n := note.n % 12
	if s[n] {
		return note
	}
	for i, j := n-1, n+1; i >= 0 || j < 12; i, j = i-1, j+1 {
		k := i
		for k < 0 {
			k += 12
		}
		if s[k] {
			note.n = i + oct*12
			break
		}
		if s[j%12] {
			note.n = j + oct*12
			break
		}
	}
	return note
}

var (
	cmaj  = Scale{true, false, true, false, true, true, false, true, false, true, false, true}
	other = Scale{true, false, true, true, false, true, false, true, false, true, true, false}
	pent  = Scale{false, true, false, true, false, false, true, false, true, false, true, false}
)
