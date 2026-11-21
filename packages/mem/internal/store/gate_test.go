package store

import (
	"context"
	"errors"
	"math"
	"testing"

	"mem/internal/text"
)

type label int

const (
	unrelated label = iota
	distinct
	faint
	related
	duplicate
)

func (l label) String() string {
	return [...]string{"unrelated", "distinct", "faint", "related", "duplicate"}[l]
}

type pair struct {
	name     string
	old, new spec
	want     label
}

var (
	enPush = spec{key: "no-push-default", title: "No git push by default",
		body: "Do not git push by default. After a commit, wait for explicit authorization; every push is its own action."}
	enBuild = spec{key: "build-before-deploy", title: "Build before deploy",
		body: "Always nix build the toplevel and verify the output before asking the user to switch."}
	enSpeakers = spec{key: "quiet-audio-tests", title: "Speakers need explicit permission",
		body: "Never play test sounds through the laptop speakers without explicit permission; DSP experiments on speakers are forbidden."}
	enVolume = spec{key: "no-volume-changes", title: "Volume untouched",
		body: "Do not change volume with wpctl, pactl or amixer without asking, even if the final level is restored."}
	ruPush = spec{key: "push-ask", title: "Не пушить по умолчанию",
		body: "После коммита не делать git push без явного разрешения; каждый пуш отдельное действие."}
	ruVolume = spec{key: "volume-ask", title: "Громкость не трогать",
		body: "Громкость через wpctl, pactl и amixer не менять без спроса, даже с сохранением итога."}
	ruBuilds = spec{key: "builds-background", title: "Длинные сборки в фоне",
		body: "Длинные сборки nix build запускать в фоне, дефолт -j4, не на батарее."}
	ruKLC = spec{key: "klc-0b-bricks", title: "KLC 0b кирпичит",
		body: "На SMOK никогда не писать KLC 0b: это кирпичит плату 1122."}
	enTouchpad = spec{key: "smok-touchpad-strip-heat", title: "SMOK touchpad strip heats the glass",
		body: "The light bar next to the trackpad warms the glass to 41 degrees after an hour of gaming; a thermal camera shows the hot spot left of the hinge, and the fans do not react to it."}
)

var gatePairs = []pair{
	{"en push reworded", enPush, spec{key: "never-push-unasked", title: "Never push without asking",
		body: "Never push to the remote by default: after committing, wait until the user explicitly authorizes the push."}, duplicate},
	{"en build reworded", enBuild, spec{key: "build-then-switch", title: "Build first, then switch",
		body: "Before asking for a switch, run nix build of the toplevel and check the output."}, duplicate},
	{"en speakers reworded", enSpeakers, spec{key: "speaker-permission", title: "No speaker tests without permission",
		body: "Playing test audio on the speakers requires explicit permission. DSP experiments on the speakers are not allowed."}, duplicate},
	{"en volume reworded", enVolume, spec{key: "leave-volume", title: "Leave the volume alone",
		body: "Never touch the volume (wpctl, pactl, amixer) without asking first, even when restoring the final level afterwards."}, duplicate},
	{"en push contradiction", enPush, spec{key: "push-freely", title: "Git push by default",
		body: "Git push by default after a commit, no explicit authorization needed for each push."}, duplicate},
	{"ru push reworded", ruPush, spec{key: "push-permission", title: "Пуш только с разрешения",
		body: "Не пушить по умолчанию: после коммита ждать явного разрешения на git push."}, duplicate},
	{"ru volume reworded", ruVolume, spec{key: "volume-alone", title: "Не менять громкость",
		body: "Без спроса громкость не трогать: wpctl, pactl, amixer запрещены, даже если потом вернуть итоговый уровень."}, duplicate},
	{"ru builds reworded", ruBuilds, spec{key: "builds-fon", title: "Сборки запускать фоном",
		body: "Долгие сборки nix build всегда запускать в фоне с -j4; на батарее сборки не запускать."}, duplicate},
	{"ru klc reworded", ruKLC, spec{key: "klc-ban", title: "Запрет KLC 0b на SMOK",
		body: "KLC 0b на SMOK не записывать, плата 1122 превращается в кирпич."}, duplicate},
	{"en fork push exception", enPush, spec{key: "freetoken-fork-push", title: "FreeToken fork push allowed",
		body: "Pushing to the FreeToken fork is allowed without asking."}, faint},
	{"en per-change rebuild", enBuild, spec{key: "no-rebuild-per-change", title: "No rebuild per change",
		body: "Do not rebuild every host after each edit; evaluate and build all hosts once at the end."}, faint},
	{"en speaker volume", enSpeakers, spec{key: "speaker-volume-cap", title: "Speaker volume cap",
		body: "Keep the speaker volume under 40 percent during calls."}, faint},
	{"ru strata fork push", ruPush, spec{key: "strata-fork", title: "Форк Strata: пуш без спроса",
		body: "В форк Strata коммитить и пушить можно без спроса, nixos-config не пушить."}, faint},
	{"ru builds while model runs", ruBuilds, spec{key: "maya-ram", title: "Не собирать пока крутится модель",
		body: "Пока работает модель Maya, сборки nix build не запускать: оперативная память занята."}, related},
	{"en same subject in the titles", enTouchpad, spec{key: "smok-touchpad-strip-power", title: "SMOK touchpad strip power states",
		body: "On wall power the strip under the palm rest stays lit in every profile; the embedded controller dims it only on battery below 20 percent, and the vendor tool has no switch for it."}, related},
	{"en firmware vs push", enPush, spec{key: "samsung-firmware", title: "Samsung firmware 1B2QNXH7",
		body: "Never install Samsung firmware 1B2QNXH7 on KURWA or the 9100 Pro."}, unrelated},
	{"en zfs vs speakers", enSpeakers, spec{key: "zfs-readahead", title: "ZFS readahead patches",
		body: "Own ZFS readahead patches run in production on SMOK; do not run mmapcheck deadlock on the host."}, unrelated},
	{"en build vs volume", enVolume, spec{key: "attic-cache", title: "Attic cache",
		body: "The attic binary cache on BIMBA serves the store paths over Tailscale."}, unrelated},
	{"ru tailscale vs volume", ruVolume, spec{key: "tailscale-buffers", title: "Буферы Tailscale",
		body: "Tailscale socketBufferSize поднят с 7 до 16 МБ, иначе UDP теряет пакеты."}, unrelated},
	{"ru builds vs klc", ruKLC, spec{key: "long-builds", title: "Длинные сборки в фоне",
		body: "Длинные сборки nix build запускать в фоне, дефолт -j4, не на батарее."}, unrelated},
	{"ru push vs volume", ruPush, spec{key: "audio-levels", title: "Громкость не трогать",
		body: "Громкость через wpctl, pactl и amixer не менять без спроса, даже с сохранением итога."}, unrelated},
	{"en build vs flake update", enBuild, spec{key: "flake-update-on-ask", title: "Flake inputs update",
		body: "Run nix flake update only when asked; the lock file pins every input."}, distinct},
	{"en push vs commit titles", enPush, spec{key: "commit-title-only", title: "Commit titles only",
		body: "Commit messages are a single title line in Conventional Commits style."}, distinct},
	{"en speakers vs bluetooth", enSpeakers, spec{key: "xm6-multipoint", title: "XM6 multipoint drop",
		body: "A silent stream to the Bluetooth sink makes the XM6 headphones drop the laptop after 20 seconds."}, distinct},
	{"ru volume vs model start", ruVolume, spec{key: "model-start-ram", title: "Модель без спроса не запускать",
		body: "Модель на SMOK без спроса не запускать: система виснет при нехватке памяти."}, distinct},
	{"ru klc vs bios version", ruKLC, spec{key: "smok-bios", title: "BIOS SMOK 50E",
		body: "На SMOK стоит BIOS 50E, новее публичной 50D, обновлений прошивок нет."}, distinct},
	{"ru builds vs battery display", ruBuilds, spec{key: "battery-display", title: "Тесты на батарее",
		body: "На батарее тесты гонять только с включённым дисплеем."}, distinct},
}

func similarity(a, b spec) float64 {
	return text.NewSet(RecordText(Record{Key: a.key, Title: a.title, Body: a.body})).
		Similarity(text.NewSet(RecordText(Record{Key: b.key, Title: b.title, Body: b.body})))
}

func listed(a, b spec) bool {
	topic := text.NewSet(topicText(Record{Key: a.key, Title: a.title})).Intersect(text.NewSet(topicText(Record{Key: b.key, Title: b.title})))
	return DefaultGate().Listed(Candidate{Similarity: similarity(a, b), Topic: topic})
}

const gateMargin = 0.05

func TestGateThresholdsSeparateFixtures(t *testing.T) {
	lo := map[label]float64{}
	hi := map[label]float64{}
	for _, l := range []label{unrelated, distinct, faint, related, duplicate} {
		lo[l], hi[l] = math.Inf(1), math.Inf(-1)
	}
	for _, p := range gatePairs {
		s, shown := similarity(p.old, p.new), listed(p.old, p.new)
		t.Logf("%-30s %-9s %.3f listed %v", p.name, p.want, s, shown)
		lo[p.want] = math.Min(lo[p.want], s)
		hi[p.want] = math.Max(hi[p.want], s)
		ok := true
		switch p.want {
		case duplicate:
			ok = s >= DefaultDup
		case related:
			ok = shown && s < DefaultDup
		case faint, distinct, unrelated:
			ok = !shown && s < DefaultDup
		}
		if !ok {
			t.Errorf("%s: similarity %.3f, listed %v does not fit %s under dup %.2f rel %.2f", p.name, s, shown, p.want, DefaultDup, DefaultRel)
		}
	}
	t.Logf("duplicate [%.3f, %.3f] related [%.3f, %.3f] distinct [%.3f, %.3f] unrelated [%.3f, %.3f]",
		lo[duplicate], hi[duplicate], lo[related], hi[related], lo[distinct], hi[distinct], lo[unrelated], hi[unrelated])
	notDup := math.Max(hi[related], math.Max(hi[distinct], hi[faint]))
	if DefaultDup-notDup < gateMargin || lo[duplicate]-DefaultDup < gateMargin {
		t.Errorf("dup threshold %.2f is within %.2f of a fixture: non-duplicates up to %.3f, duplicates from %.3f",
			DefaultDup, gateMargin, notDup, lo[duplicate])
	}
	if RelFloor <= hi[unrelated] || lo[related] < RelFloor {
		t.Errorf("rel floor %.2f must lie in (%.3f, %.3f]", RelFloor, hi[unrelated], lo[related])
	}
}

func TestGate(t *testing.T) {
	ctx := context.Background()
	for _, p := range gatePairs {
		t.Run(p.name, func(t *testing.T) {
			s, _ := newStore(t)
			mustPut(t, s, p.old.req())
			res, err := s.Put(ctx, p.new.req())
			var ge *GateError
			switch p.want {
			case duplicate:
				if !errors.As(err, &ge) {
					t.Fatalf("want GateError, got %v", err)
				}
				if len(ge.Duplicates) != 1 || ge.Duplicates[0].Record.Key != p.old.key {
					t.Fatalf("duplicates = %+v, want %s", ge.Duplicates, p.old.key)
				}
				if _, err := s.Get(ctx, p.new.key); !errors.Is(err, ErrNotFound) {
					t.Fatalf("rejected record was stored: %v", err)
				}
			case related:
				if err != nil {
					t.Fatalf("put: %v", err)
				}
				if len(res.Related) != 1 || res.Related[0].Record.Key != p.old.key {
					t.Fatalf("related = %+v, want %s", res.Related, p.old.key)
				}
			case faint, distinct, unrelated:
				if err != nil {
					t.Fatalf("put: %v", err)
				}
				if len(res.Related) != 0 {
					t.Fatalf("related = %+v, want none", res.Related)
				}
			}
		})
	}
}

func TestGateMinShared(t *testing.T) {
	ctx := context.Background()
	cases := []struct {
		name      string
		old, new  spec
		minShared int
		dup       bool
	}{
		{"one shared stem", spec{key: "core-flag", body: "Core."}, spec{key: "not-core", body: "Not in core."}, 0, false},
		{"one shared stem, floor lowered", spec{key: "core-flag", body: "Core."}, spec{key: "not-core", body: "Not in core."}, 1, true},
		{"two shared stems", spec{key: "smok-bios", body: "SMOK BIOS."}, spec{key: "bios-smok", body: "BIOS on SMOK."}, 0, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s, _ := newStore(t)
			mustPut(t, s, c.old.req())
			req := c.new.req()
			req.Gate.MinShared = c.minShared
			_, err := s.Put(ctx, req)
			var ge *GateError
			if got := errors.As(err, &ge); got != c.dup {
				t.Fatalf("duplicate = %v (%v), want %v", got, err, c.dup)
			}
		})
	}
}
