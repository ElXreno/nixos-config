package hook

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"mem/internal/store"
)

func (sb *sandbox) seedWith(t *testing.T, recs []store.Record) {
	t.Helper()
	ctx := context.Background()
	s, err := store.Open(ctx, sb.dbPath(), store.Options{Now: func() time.Time { return seedTime }})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()
	if _, err := s.Upsert(ctx, recs, nil, "seed"); err != nil {
		t.Fatal(err)
	}
}

func promptInput(t *testing.T, sb *sandbox, session, prompt string) string {
	t.Helper()
	data, err := json.Marshal(map[string]string{"session_id": session, "cwd": sb.root + "/repos/nixos-config",
		"hook_event_name": "UserPromptSubmit", "prompt": prompt})
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func numbered(n int, format string) []string {
	out := make([]string, n)
	for i := range out {
		out[i] = fmt.Sprintf(format, i+1)
	}
	return out
}

func TestPromptPacking(t *testing.T) {
	sb := newSandbox(t)
	apst := numbered(40, "Note %02d on the BIMBA board: firmware, cabling and slot notes for drive bay, nothing about power states.")
	apst[32] = "APST stays off on BIMBA: nvme_core.default_ps_max_latency_us=0 keeps the 990 Pro from dropping off the bus under load."
	sb.seedWith(t, []store.Record{
		{Key: "apst-990pro", Kind: store.Fact, Scopes: []string{"global"}, Title: "Samsung 990 Pro on BIMBA: APST and power states",
			Body: strings.Join(apst, "\n")},
		{Key: "smart-990pro", Kind: store.Fact, Scopes: []string{"global"}, Title: "Samsung 990 Pro SMART log on BIMBA, APST",
			Body: strings.Join(numbered(22, "SMART entry %02d for the 990 Pro on BIMBA: media errors 0, wear within limits, keep watching it."), "\n")},
	})
	prompt := "does the 990 Pro on BIMBA still need default_ps_max_latency_us=0 to keep APST off?"
	sb.check(t, spec{name: "the top record is cut, not dropped", event: UserPromptSubmit, stdin: promptInput(t, sb, "p1", prompt),
		has: []string{"`apst-990pro` [fact", "`smart-990pro` [fact", "default_ps_max_latency_us=0 keeps the 990 Pro from dropping off the bus",
			"Note 01 on the BIMBA board", "`mem get apst-990pro` prints the whole record"},
		log: []string{"inject", "apst-990pro="}})
	_, out, _ := sb.run(promptInput(t, sb, "p2", prompt), UserPromptSubmit)
	var env envelope
	if err := json.Unmarshal([]byte(out), &env); err != nil {
		t.Fatalf("%v: %s", err, out)
	}
	text := env.HookSpecificOutput.AdditionalContext
	if n := len([]rune(text)); n > 2500 {
		t.Errorf("context of %d chars over the 2500 budget", n)
	}
	if i, j := strings.Index(text, "`apst-990pro`"), strings.Index(text, "`smart-990pro`"); i < 0 || j < i {
		t.Errorf("records out of rank order:\n%s", text)
	}
}

func fact(key, title, body string) store.Record {
	return store.Record{Key: key, Kind: store.Fact, Scopes: []string{"global"}, Title: title, Body: body}
}

var gateCaseRecords = []store.Record{
	fact("reference_bs3pro_command_0xdf_bricks", "Flydigi BS3 Pro command 0xDF erases the firmware and reboots to the ROM ISP loader - never send it",
		"Never send command 0xDF to a Flydigi BS3 Pro. 0xDF erases the first flash sector and resets the MCU into the ROM ISP bootloader; "+
			"the cooler stays dead until it is reflashed over USB. Flydigi has no unlock or payload gate: the bare 0xDF byte does it over HID or BLE."),
	fact("reference_ugreen_cd226_charger", "Ugreen Nexode CD226 charger, per-port limits and the shared power budget",
		"Выдёргивание соседнего устройства из зарядки роняет BT-линк кулера Flydigi."),
	fact("reference_fan_bearing_lubricant", "Чем смазывать втулку вентилятора и почему",
		"Почему вентилятор шумит: высохла смазка спечённой бронзовой втулки. Силикон отвергнут, он не смешивается с мыльным загустителем; "+
			"WD-40 вытесняет остатки масла, литиевая смазка густеет на холоде, а минеральное масло ISO VG 32 подходит лучше всего."),
	fact("reference_akko_5087s_skus", "Akko 5087S: почему QMK-ревизия рискованна на Linux",
		"Прошивальщик для этой платы в nixpkgs отсутствует."),
	{Key: "feedback_fio_cold_cache", Kind: store.Rule, Scopes: []string{"global"},
		Title: "fio benchmarks need explicit writes; --create_only=1 leaves files sparse",
		Body:  "A test file that fits in RAM measures the page cache. Sparse files read back as zeros without disk IO."},
	fact("reference_stylix_scheme_attrset_variant", "A stylix base16Scheme attrset needs variant dark or Zed silently falls back to One Dark",
		"Zed writes the theme as unspecified and falls back without an error."),
	fact("reference_postgres_btrfs_cow", "Postgres on btrfs needs nodatacow",
		"Copy-on-write fragments the write-ahead log of the database."),
	fact("reference_libpq_socket_url", "libpq socket URLs",
		"An empty host leaves libpq on the default socket path; it falls back to TCP only when asked."),
	fact("reference_mc_scheduler_starvation", "Minecraft server tick scheduler starvation",
		"The scheduler leaves the tick thread starved when a mod writes chunks synchronously; TPS falls until the worker threads park."),
}

func TestPromptGateCases(t *testing.T) {
	sb := newSandbox(t)
	sb.seedWith(t, gateCaseRecords)
	danger := "почему вентилятор flydigi кулера нельзя прошивать командой 0xDF"
	haiku := "write a haiku about autumn leaves falling in the park"
	sb.check(t, spec{name: "default gate: danger prompt in Russian through the lead rule", event: UserPromptSubmit,
		stdin: promptInput(t, sb, "g1", danger),
		has:   []string{"`reference_bs3pro_command_0xdf_bricks` [fact", "Never send command 0xDF"}, lacks: []string{"ugreen"},
		log: []string{"reference_bs3pro_command_0xdf_bricks=0.286/2/2"}})
	sb.check(t, spec{name: "default gate: unrelated prompt", event: UserPromptSubmit,
		stdin: promptInput(t, sb, "g2", haiku), empty: true, log: []string{"session=g2 none"}})
	knee := []string{"--min-topic", "0", "--lead-shared", "3"}
	sb.check(t, spec{name: "the sweep knee misses the danger prompt", event: UserPromptSubmit, args: knee,
		stdin: promptInput(t, sb, "g3", danger), empty: true})
	sb.check(t, spec{name: "the sweep knee injects on the haiku", event: UserPromptSubmit, args: knee,
		stdin: promptInput(t, sb, "g4", haiku), has: []string{"reference_mc_scheduler_starvation"}})
	stageA := []string{"--min-cover", "0.3", "--min-shared", "2", "--min-topic", "1", "--lead=false"}
	sb.check(t, spec{name: "the stage A gate misses the danger prompt", event: UserPromptSubmit, args: stageA,
		stdin: promptInput(t, sb, "g5", danger), empty: true})
	sb.check(t, spec{name: "the stage A gate injects on the haiku", event: UserPromptSubmit, args: stageA,
		stdin: promptInput(t, sb, "g6", haiku), has: []string{"feedback_fio_cold_cache"}})
}

func TestMinSharedFlag(t *testing.T) {
	sb := newSandbox(t)
	sb.seed(t)
	sb.check(t, spec{name: "--min-shared 0", event: UserPromptSubmit, fixture: "prompt-push", args: []string{"--min-shared", "0"},
		has: []string{"`no-push-default` [rule"}, log: []string{"inject"}})
}
