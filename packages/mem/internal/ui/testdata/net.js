"use strict";
const vm = require("vm");
const fs = require("fs");
const util = require("util");
const [jsPath, casesPath] = process.argv.slice(2);
const cases = JSON.parse(fs.readFileSync(casesPath, "utf8"));
let clock = 0,
  seq = 0;
const timers = new Map();
const sandbox = {
  Math,
  Map,
  Set,
  Array,
  Object,
  String,
  Number,
  JSON,
  Error,
  setTimeout: (f, d) => {
    const id = ++seq;
    timers.set(id, { f, at: clock + d });
    return id;
  },
  clearTimeout: (id) => {
    timers.delete(id);
  },
};
sandbox.window = sandbox;
vm.createContext(sandbox);
vm.runInContext(fs.readFileSync(jsPath, "utf8"), sandbox);
const N = sandbox.MemGfx.net;
const advance = (ms) => {
  const end = clock + ms;
  for (;;) {
    let best = null,
      bid = 0;
    for (const [id, t] of timers)
      if (t.at <= end && (!best || t.at < best.at)) {
        best = t;
        bid = id;
      }
    if (!best) break;
    timers.delete(bid);
    clock = best.at;
    best.f();
  }
  clock = end;
};
class FakeES {
  constructor(url) {
    this.url = url;
    this.readyState = 0;
    this.l = {};
    this.closed = false;
    FakeES.all.push(this);
  }
  addEventListener(n, f) {
    this.l[n] = f;
  }
  close() {
    this.closed = true;
    this.readyState = 2;
  }
  open() {
    this.readyState = 1;
    if (this.onopen) this.onopen({});
  }
  error(closed) {
    this.readyState = closed ? 2 : 0;
    if (this.onerror) this.onerror({});
  }
  emit(n, data, id) {
    if (this.l[n]) this.l[n]({ data, lastEventId: id });
  }
}
FakeES.all = [];
const checks = {};
const ok = (name, v) => {
  checks[name] = !!v;
};

cases.forEach((c, i) => {
  let got = null;
  try {
    got = N.applyDiff(JSON.parse(JSON.stringify(c.old)), c.diff);
  } catch (e) {
    got = null;
  }
  ok(
    "apply." + c.name,
    got && util.isDeepStrictEqual(JSON.parse(JSON.stringify(got)), c.full),
  );
  ok(
    "apply.keepsInput." + c.name,
    (() => {
      const o = JSON.parse(JSON.stringify(c.old));
      const before = JSON.stringify(o);
      try {
        N.applyDiff(o, c.diff);
      } catch (e) {
        return false;
      }
      return JSON.stringify(o) === before;
    })(),
  );
});
{
  const c = cases[0];
  const bad = JSON.parse(JSON.stringify(c.diff));
  bad.topics = bad.topics.map((t) => ({ id: t.id + "_x", same: true }));
  let threw = false;
  try {
    N.applyDiff(c.old, bad);
  } catch (e) {
    threw = true;
  }
  ok("apply.throwsOnUnknownSame", threw);
  threw = false;
  try {
    N.applyDiff(null, c.diff);
  } catch (e) {
    threw = true;
  }
  ok("apply.throwsOnNull", threw);
}

const harness = (extra = {}) => {
  const log = { modes: [], graphs: [], turns: [], opens: 0 };
  FakeES.all = [];
  const net = N.create(
    Object.assign(
      {
        EventSource: FakeES,
        setTimeout: sandbox.setTimeout,
        clearTimeout: sandbox.clearTimeout,
        onGraph: (r) => log.graphs.push(r),
        onTurn: (t, id) => log.turns.push([t, id]),
        onOpen: () => {
          log.opens++;
        },
        onMode: (m) => log.modes.push(m),
      },
      extra,
    ),
  );
  return { net, log };
};
const last = () => FakeES.all[FakeES.all.length - 1];

{
  const { net, log } = harness({ EventSource: undefined });
  net.start();
  ok(
    "net.noEventSourceFallsBack",
    net.mode() === "poll" && !net.live() && log.modes.join() === "poll",
  );
}
{
  const { net, log } = harness();
  net.start();
  const es = last();
  ok("net.url", es.url === "/api/events");
  ok("net.notLiveBeforeOpen", !net.live() && net.mode() === "off");
  es.open();
  ok(
    "net.liveAfterOpen",
    net.live() && log.opens === 1 && log.modes.join() === "sse",
  );
  es.emit("graph", '{"rev":"abc"}');
  es.emit("graph", "nonsense");
  es.emit("graph", '{"rev":""}');
  es.emit("graph", '{"rev":5}');
  es.emit("graph", "{}");
  ok("net.graphEvent", log.graphs.join() === "abc");
  es.emit("turn", '{"id":"t1","session":"s"}', "cur1");
  es.emit("turn", "x");
  es.emit("turn", "5");
  es.emit("turn", "null");
  ok(
    "net.turnEvent",
    log.turns.length === 1 &&
      log.turns[0][0].id === "t1" &&
      log.turns[0][1] === "cur1",
  );
  es.error(false);
  ok(
    "net.oneErrorKeepsSse",
    net.mode() === "sse" && !es.closed && net.errors() === 1,
  );
  es.open();
  ok("net.openResetsErrors", net.errors() === 0);
  es.error(false);
  es.error(false);
  ok(
    "net.twoErrorsFallBack",
    net.mode() === "poll" && es.closed && log.modes.join() === "sse,poll",
  );
  es.emit("graph", '{"rev":"stale"}');
  ok("net.closedSourceIsIgnored", log.graphs.join() === "abc");
  const n = FakeES.all.length;
  advance(N.RETRY_MS - 1);
  ok("net.retryWaits", FakeES.all.length === n);
  advance(2);
  ok(
    "net.retryReconnects",
    FakeES.all.length === n + 1 && last().url === "/api/events",
  );
  last().open();
  ok(
    "net.recoversToSse",
    net.mode() === "sse" &&
      log.modes.join() === "sse,poll,sse" &&
      log.opens === 3,
  );
  net.stop();
  ok("net.stopCloses", last().closed && net.mode() === "off");
  const m = FakeES.all.length;
  advance(N.RETRY_MS * 3);
  ok("net.stopStaysStopped", FakeES.all.length === m);
}
{
  const { net, log } = harness();
  net.start();
  last().error(true);
  ok(
    "net.closedSourceFallsBackAtOnce",
    net.mode() === "poll" && last().closed && log.modes.join() === "poll",
  );
  net.stop();
}
{
  const { net, log } = harness();
  net.start();
  advance(N.OPEN_MS - 1);
  ok("net.openWaits", net.mode() === "off");
  advance(2);
  ok(
    "net.openTimeoutFallsBack",
    net.mode() === "poll" && last().closed && log.modes.join() === "poll",
  );
  net.stop();
}
{
  const { net, log } = harness();
  net.start();
  last().open();
  const first = last();
  net.hide();
  advance(N.HIDDEN_MS - 1);
  ok("net.hideWaits", net.live() && !first.closed);
  advance(2);
  ok("net.hideReleasesConnection", first.closed && net.mode() === "poll");
  net.show();
  ok("net.showReconnects", last() !== first && !last().closed);
  last().open();
  ok("net.showGoesLive", net.live() && log.modes.join() === "sse,poll,sse");
  net.hide();
  net.show();
  advance(N.HIDDEN_MS * 2);
  ok("net.showCancelsHide", net.live() && !last().closed);
  net.stop();
}
{
  const { net } = harness({ hidden: () => true });
  net.start();
  last().open();
  last().error(true);
  const n = FakeES.all.length;
  advance(N.RETRY_MS * 2);
  ok("net.noRetryWhileHidden", FakeES.all.length === n);
  net.stop();
}
process.stdout.write(JSON.stringify(checks));
