package main

import (
	"flag"
	"fmt"
	"log"
	"math/rand"
	"os"
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

	quit := make(chan bool)
	go func() {
		b := make([]byte, 1)
		os.Stdin.Read(b)
		quit <- true
	}()

	notes := make(chan Note)
	go gen(notes)

	for {
		select {
		case <-quit:
			return nil
		case n, ok := <-notes:
			if !ok {
				return nil
			}
			if n.n > 0 {
				s.WriteShort(0x94, n.n, 100)
			}
			q := false
			select {
			case <-quit:
				q = true
			case <-time.After(n.d):
			}
			if n.n > 0 {
				s.WriteShort(0x84, n.n, 100)
			}
			if q {
				return nil
			}
		}
	}
}

type Note struct {
	n int64
	d time.Duration
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

func genX(notes chan Note) {
	for {
		notes <- Note{60, 50 * time.Millisecond}
		notes <- Note{0, 50 * time.Millisecond}
		notes <- Note{72, 50 * time.Millisecond}
		notes <- Note{0, 50 * time.Millisecond}
	}
}

func gen(notes chan Note) {
	ring := make([]Note, 8)
	for i := range ring {
		ring[i] = Note{60, 50 * time.Millisecond}
	}
	for {
		for i := range ring {
			ring[i] = mutate(ring[i])
			notes <- ring[i]
			notes <- Note{0, 50 * time.Millisecond}
		}
	}
}

var (
	cmaj = Scale{true, false, true, false, true, true, false, true, false, true, false, true}
)

func mutate(n Note) Note {
	if rand.Intn(4) == 0 {
		n.n += int64(rand.Intn(13) - 6)
		n = cmaj.Quantize(n)
		if n.n < 36 {
			n.n = 36
		}
		if n.n > 84 {
			n.n = 84
		}
	}
	if rand.Intn(4) == 0 {
		n.d += time.Duration(rand.Intn(3)-1) * 50 * time.Millisecond
		if n.d < 50*time.Millisecond {
			n.d = 50 * time.Millisecond
		}
		if n.d > 500*time.Millisecond {
			n.d = 500 * time.Millisecond
		}
	}
	return n
}
