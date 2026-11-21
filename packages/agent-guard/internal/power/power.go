package power

import (
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
)

const (
	Check       = "power"
	DefaultRoot = "/sys/class/power_supply"
)

type State struct {
	Battery  bool
	OnAC     bool
	Capacity int
	Status   string
}

func (s State) OnBattery() bool { return s.Battery && !s.OnAC }

func (s State) at() string {
	if s.Capacity < 0 {
		return ""
	}
	return " at " + strconv.Itoa(s.Capacity) + "%"
}

func (s State) String() string {
	if !s.Battery {
		return "AC, no battery"
	}
	out := "battery"
	if s.OnAC {
		out = "AC, battery"
	}
	if s.Capacity >= 0 {
		out += " " + strconv.Itoa(s.Capacity) + "%"
	}
	if s.Status != "" {
		out += ", " + strings.ToLower(s.Status)
	}
	return out
}

type battery struct {
	status                string
	capacity              int
	energyNow, energyFull int64
	chargeNow, chargeFull int64
}

func Read(root string) State {
	s := State{OnAC: true, Capacity: -1}
	if root == "" {
		return s
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		return s
	}
	var bats []battery
	external, online, charging, discharging := false, false, false, false
	for _, e := range entries {
		dir := filepath.Join(root, e.Name())
		if attr(dir, "scope") == "Device" {
			continue
		}
		switch t := attr(dir, "type"); {
		case t == "Battery":
			if attr(dir, "present") == "0" {
				continue
			}
			b := readBattery(dir)
			bats = append(bats, b)
			charging = charging || b.status == "Charging"
			discharging = discharging || b.status == "Discharging"
		case t == "Mains" || t == "Wireless" || t == "BrickID" || strings.HasPrefix(t, "USB"):
			external = true
			if v := attr(dir, "online"); v != "" && v != "0" {
				online = true
			}
		}
	}
	if len(bats) == 0 {
		return s
	}
	s.Battery = true
	if external {
		s.OnAC = online || charging
	} else {
		s.OnAC = !discharging
	}
	s.Capacity = capacity(bats)
	s.Status = bats[0].status
	for _, b := range bats {
		if b.status == "Discharging" || (b.status == "Charging" && s.Status != "Discharging") {
			s.Status = b.status
		}
	}
	return s
}

func attr(dir, name string) string {
	b, err := os.ReadFile(filepath.Join(dir, name))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}

func number(dir, name string) int64 {
	n, err := strconv.ParseInt(attr(dir, name), 10, 64)
	if err != nil {
		return -1
	}
	return n
}

func readBattery(dir string) battery {
	c := int(number(dir, "capacity"))
	if c > 100 {
		c = 100
	}
	return battery{
		status:     attr(dir, "status"),
		capacity:   c,
		energyNow:  number(dir, "energy_now"),
		energyFull: number(dir, "energy_full"),
		chargeNow:  number(dir, "charge_now"),
		chargeFull: number(dir, "charge_full"),
	}
}

func capacity(bats []battery) int {
	if len(bats) == 1 && bats[0].capacity >= 0 {
		return bats[0].capacity
	}
	if v, ok := ratio(bats, func(b battery) (int64, int64) { return b.energyNow, b.energyFull }); ok {
		return v
	}
	if v, ok := ratio(bats, func(b battery) (int64, int64) { return b.chargeNow, b.chargeFull }); ok {
		return v
	}
	sum, n := 0, 0
	for _, b := range bats {
		if b.capacity >= 0 {
			sum, n = sum+b.capacity, n+1
		}
	}
	if n == 0 {
		return -1
	}
	return int(math.Round(float64(sum) / float64(n)))
}

func ratio(bats []battery, get func(battery) (int64, int64)) (int, bool) {
	var now, full int64
	for _, b := range bats {
		n, f := get(b)
		if n < 0 || f <= 0 {
			return 0, false
		}
		now, full = now+n, full+f
	}
	return min(100, int(math.Round(100*float64(now)/float64(full)))), true
}

type Probe struct {
	Root  string
	once  sync.Once
	state State
}

func (p *Probe) State() State {
	p.once.Do(func() { p.state = Read(p.Root) })
	return p.state
}

func HeavyNote(s State, p Pattern) string {
	return "Power: on battery" + s.at() + ", heavy build (`" + p.shown + "`). If it can wait, `agent-guard power wait ac` run in the background exits once AC is back."
}

func Change(prev State, seen bool, cur State) string {
	switch {
	case !seen && cur.OnBattery():
		return "Power: on battery" + cur.at() + "."
	case !seen || prev.OnBattery() == cur.OnBattery():
		return ""
	case cur.OnBattery():
		return "Power: unplugged, now on battery" + cur.at() + "."
	}
	if cur.Capacity >= 0 {
		return "Power: back on AC, battery" + cur.at() + "."
	}
	return "Power: back on AC."
}
