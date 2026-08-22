document.addEventListener("alpine:init", () => {
  Alpine.data("catalog", () => ({
    stories: window.__GLYPH_STORIES__ || [],
    mode: "plain",
    si: 0,
    ni: 0,
    inspect: { xy: "—", ch: "—", fg: "—", bg: "—", st: "—" },
    get groups() {
      const g = {};
      this.stories.forEach((s, i) => {
        const k = s.feature || "ungrouped";
        (g[k] || (g[k] = [])).push({ s, i });
      });
      return Object.keys(g)
        .sort()
        .map((feature) => ({ feature, items: g[feature] }));
    },
    get current() {
      return this.stories[this.si] || null;
    },
    get snap() {
      const s = this.current;
      if (!s || !s.snapshots || !s.snapshots.length) return null;
      const i = Math.max(0, Math.min(this.ni, s.snapshots.length - 1));
      return s.snapshots[i];
    },
    get svg() {
      const snap = this.snap;
      if (!snap || snap.status !== "ok") return "";
      if (this.mode === "grid") return snap.svgGrid || snap.svgPlain || "";
      if (this.mode === "spaces") return snap.svgSpaces || snap.svgPlain || "";
      return snap.svgPlain || "";
    },
    get windowTitle() {
      const s = this.current;
      if (!s) return "";
      return s.name + (s.runId ? " · " + s.runId : "");
    },
    get size() {
      const snap = this.snap;
      if (!snap) return "";
      return (snap.cols || 0) + "×" + (snap.rows || 0);
    },
    get emptyMsg() {
      const s = this.current;
      if (!s) return "select a story";
      const snap = this.snap;
      if (!snap) {
        return s.status === "not_run" ? "not run — glyph run " + s.name : s.status;
      }
      if (snap.status !== "ok") return "unreadable snapshot " + (snap.error || "");
      return "";
    },
    get inspectRows() {
      return [
        { k: "cell", v: this.inspect.xy },
        { k: "char", v: this.inspect.ch },
        { k: "fg", v: this.inspect.fg },
        { k: "bg", v: this.inspect.bg },
        { k: "style", v: this.inspect.st },
        { k: "keys", v: "j k stories · [ ] snaps · 1 2 3 mode" },
      ];
    },
    select(i, n) {
      this.si = i;
      this.ni = n || 0;
    },
    origin() {
      return this.mode === "plain" ? { x: 10, y: 10 } : { x: 38, y: 26 };
    },
    hover(ev) {
      const svg = ev.currentTarget.querySelector("svg");
      if (!svg || !this.snap) return;
      const r = svg.getBoundingClientRect();
      const vb = svg.viewBox.baseVal;
      const o = this.origin();
      const x = Math.floor(((ev.clientX - r.left) * (vb.width / r.width) - o.x) / 10);
      const y = Math.floor(((ev.clientY - r.top) * (vb.height / r.height) - o.y) / 20);
      const cell = (this.snap.cells || []).find((c) => c.x === x && c.y === y);
      const raw = cell && cell.char ? cell.char : " ";
      const shown = raw === " " ? "·" : raw;
      const st = cell && cell.style ? cell.style : {};
      const attrs = [st.bold && "bold", st.dim && "dim", st.italic && "italic", st.underline && "underline", st.reverse && "reverse"]
        .filter(Boolean)
        .join(" ") || "none";
      this.inspect = {
        xy: x + "," + y,
        ch: JSON.stringify(shown),
        fg: st.fg || "default",
        bg: st.bg || "default",
        st: attrs + (cell && cell.link ? " · " + cell.link : ""),
      };
    },
    onKey(ev) {
      const n = this.stories.length;
      if (ev.key === "j" || ev.key === "ArrowDown") {
        this.select(Math.min(this.si + 1, n - 1), 0);
        ev.preventDefault();
      }
      if (ev.key === "k" || ev.key === "ArrowUp") {
        this.select(Math.max(this.si - 1, 0), 0);
        ev.preventDefault();
      }
      if ((ev.key === "]" || ev.key === "ArrowRight") && this.current && this.current.snapshots) {
        this.ni = Math.min(this.ni + 1, this.current.snapshots.length - 1);
      }
      if (ev.key === "[" || ev.key === "ArrowLeft") {
        this.ni = Math.max(this.ni - 1, 0);
      }
      if (ev.key === "1") this.mode = "plain";
      if (ev.key === "2") this.mode = "grid";
      if (ev.key === "3") this.mode = "spaces";
    },
  }));
});
