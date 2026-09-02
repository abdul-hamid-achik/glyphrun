// glyph stories inspect page.
//
// renderSVG is a port of internal/render/svg.go so the page paints every
// screen from its cell grid: one payload per snapshot, all overlays (grid,
// rulers, regions, spaces, diff) computed here. Keep the geometry in sync with
// render.DefaultOptions (cell 10×20, font 15, padding 10, ruler pads 28/16).

(function () {
  const GEO = { cw: 10, ch: 20, font: 15, pad: 10, rulerX: 28, rulerY: 16 };
  const DIFF_COLOR = "#ff5f56";

  function esc(s) {
    return String(s).replace(/&/g, "&amp;").replace(/</g, "&lt;").replace(/>/g, "&gt;").replace(/"/g, "&quot;");
  }

  function resolveColor(value, palette, fallback) {
    if (!value) return fallback;
    if (value[0] === "#") return value;
    const hex = palette.colors && palette.colors[String(value).toLowerCase()];
    return hex || fallback;
  }

  function effectiveStyle(cell, palette) {
    const st = (cell && cell.style) || {};
    let fg = resolveColor(st.fg, palette, palette.foreground);
    let bg = resolveColor(st.bg, palette, "");
    if (st.reverse) {
      const sfg = bg || palette.background;
      const sbg = fg || palette.foreground;
      fg = sfg;
      bg = sbg;
    }
    return { fg, bg, bold: !!st.bold, italic: !!st.italic, underline: !!st.underline, dim: !!st.dim };
  }

  function sameText(a, b) {
    return a.fg === b.fg && a.bold === b.bold && a.italic === b.italic && a.underline === b.underline && a.dim === b.dim;
  }

  function cellGetter(cells, cols, rows) {
    return function (x, y) {
      if (x < 0 || y < 0 || x >= cols || y >= rows) return { x, y, char: " ", width: 1 };
      const c = cells[y * cols + x];
      return c || { x, y, char: " ", width: 1 };
    };
  }

  // renderSVG(screen, opts) → SVG markup. screen = {cols, rows, cells, cursor}.
  function renderSVG(screen, opts, palette) {
    const cols = screen.cols > 0 ? screen.cols : 80;
    const rows = screen.rows > 0 ? screen.rows : 24;
    const { cw, ch, font, pad } = GEO;
    const rulers = !!opts.rulers;
    const rx = rulers ? GEO.rulerX : 0;
    const ry = rulers ? GEO.rulerY : 0;
    const ox = pad + rx;
    const oy = pad + ry;
    const width = pad * 2 + rx + cols * cw;
    const height = pad * 2 + ry + rows * ch;
    const at = cellGetter(screen.cells || [], cols, rows);
    const out = [];
    out.push(
      `<svg xmlns="http://www.w3.org/2000/svg" width="${width}" height="${height}" viewBox="0 0 ${width} ${height}" font-family="ui-monospace, SFMono-Regular, Menlo, Consolas, 'DejaVu Sans Mono', monospace" font-size="${font}">`
    );
    out.push(`<rect width="${width}" height="${height}" fill="${palette.background}"/>`);
    for (let y = 0; y < rows; y++) {
      let x = 0;
      while (x < cols) {
        const st = effectiveStyle(at(x, y), palette);
        if (!st.bg) {
          x++;
          continue;
        }
        const start = x;
        while (x < cols && effectiveStyle(at(x, y), palette).bg === st.bg) x++;
        out.push(`<rect x="${ox + start * cw}" y="${oy + y * ch}" width="${(x - start) * cw}" height="${ch}" fill="${st.bg}"/>`);
      }
    }
    for (let y = 0; y < rows; y++) {
      let x = 0;
      while (x < cols) {
        const st = effectiveStyle(at(x, y), palette);
        const start = x;
        let text = "";
        while (x < cols) {
          const cell = at(x, y);
          if (!sameText(effectiveStyle(cell, palette), st)) break;
          let chr = cell.char || " ";
          if (opts.spaces && chr === " ") chr = "·";
          text += chr;
          x++;
        }
        if (!opts.spaces && text.trim() === "") continue;
        const n = x - start;
        let attrs = `fill="${st.fg}"`;
        if (st.bold) attrs += ` font-weight="bold"`;
        if (st.italic) attrs += ` font-style="italic"`;
        if (st.underline) attrs += ` text-decoration="underline"`;
        if (st.dim) attrs += ` fill-opacity="0.6"`;
        out.push(
          `<text x="${ox + start * cw}" y="${oy + y * ch + font}" textLength="${n * cw}" lengthAdjust="spacingAndGlyphs" xml:space="preserve" ${attrs}>${esc(text)}</text>`
        );
      }
    }
    if (opts.grid) {
      out.push(`<g id="grid" fill="none" stroke="${palette.foreground}">`);
      const right = ox + cols * cw;
      const bottom = oy + rows * ch;
      for (let x = 0; x <= cols; x++) {
        const xx = ox + x * cw;
        out.push(`<line x1="${xx}" y1="${oy}" x2="${xx}" y2="${bottom}" stroke-width="1" stroke-opacity="${x % 10 === 0 ? 0.28 : 0.12}"/>`);
      }
      for (let y = 0; y <= rows; y++) {
        const yy = oy + y * ch;
        out.push(`<line x1="${ox}" y1="${yy}" x2="${right}" y2="${yy}" stroke-width="1" stroke-opacity="${y % 10 === 0 ? 0.28 : 0.12}"/>`);
      }
      out.push(`</g>`);
    }
    if (rulers) {
      out.push(`<g id="rulers" fill="${palette.foreground}" font-size="10">`);
      for (let x = 0; x < cols; x += 10) out.push(`<text x="${ox + x * cw}" y="${oy - 4}">${x}</text>`);
      for (let y = 0; y < rows; y++) out.push(`<text text-anchor="end" x="${ox - 4}" y="${oy + y * ch + ch - 6}">${y}</text>`);
      out.push(`</g>`);
    }
    const cur = screen.cursor;
    if (opts.cursor !== false && cur && cur.visible && cur.x >= 0 && cur.x < cols && cur.y >= 0 && cur.y < rows) {
      out.push(`<rect x="${ox + cur.x * cw}" y="${oy + cur.y * ch}" width="${cw}" height="${ch}" fill="none" stroke="${palette.cursor}" stroke-width="1"/>`);
    }
    if (opts.regions && opts.regions.length) {
      out.push(`<g id="regions" fill="none" stroke="${palette.cursor}" stroke-width="1">`);
      opts.regions.forEach((r) => {
        if (r.Width <= 0 || r.Height <= 0) return;
        out.push(`<rect x="${ox + r.X * cw}" y="${oy + r.Y * ch}" width="${r.Width * cw}" height="${r.Height * ch}" data-x="${r.X}" data-y="${r.Y}"/>`);
      });
      out.push(`</g>`);
    }
    if (opts.diff && opts.diff.length) {
      out.push(`<g id="diff" fill="${DIFF_COLOR}" fill-opacity="0.28" stroke="${DIFF_COLOR}" stroke-width="1">`);
      opts.diff.forEach((c) => out.push(`<rect x="${ox + c.x * cw}" y="${oy + c.y * ch}" width="${cw}" height="${ch}"/>`));
      out.push(`</g>`);
    }
    out.push(`</svg>`);
    return out.join("");
  }

  // expandGrid turns the compact payload (grid + style/link runs) back into
  // the row-major cell array the renderer and the hover inspector read.
  function expandGrid(grid, styles, links, cols, rows) {
    const cells = new Array(cols * rows);
    for (let y = 0; y < rows; y++) {
      const row = (grid && grid[y]) || [];
      for (let x = 0; x < cols; x++) {
        cells[y * cols + x] = { x, y, char: row[x] || " ", width: 1 };
      }
    }
    (styles || []).forEach((r) => {
      for (let i = 0; i < r.len; i++) {
        const c = cells[r.y * cols + r.x + i];
        if (c) c.style = r.style;
      }
    });
    (links || []).forEach((r) => {
      for (let i = 0; i < r.len; i++) {
        const c = cells[r.y * cols + r.x + i];
        if (c) c.link = r.url;
      }
    });
    return cells;
  }

  // Expanded grids are cached off the reactive tree (a WeakMap keyed by the
  // snapshot object) so Alpine never proxies a 2000-element array.
  const cellCache = new WeakMap();
  const goldenCache = new WeakMap();

  function cellsOf(p) {
    if (!p) return [];
    let cells = cellCache.get(p);
    if (!cells) {
      cells = expandGrid(p.grid, p.styles, p.links, p.cols, p.rows);
      cellCache.set(p, cells);
    }
    return cells;
  }

  function goldenCellsOf(p) {
    if (!p || !p.goldenGrid || !p.goldenGrid.length) return null;
    let cells = goldenCache.get(p);
    if (!cells) {
      const rows = p.goldenGrid.length;
      const cols = Math.max(p.cols, (p.goldenGrid[0] || []).length);
      cells = expandGrid(p.goldenGrid, p.goldenStyles, null, cols, rows);
      goldenCache.set(p, cells);
    }
    return cells;
  }

  window.__glyphRenderSVG = renderSVG;
  window.__glyphExpandGrid = expandGrid;

  document.addEventListener("alpine:init", () => {
    Alpine.data("catalog", () => ({
      payload: window.__GLYPH_STORIES__ || { stories: [], summary: {}, palette: {} },
      stories: (window.__GLYPH_STORIES__ && window.__GLYPH_STORIES__.stories) || [],
      summary: (window.__GLYPH_STORIES__ && window.__GLYPH_STORIES__.summary) || {},
      palette: (window.__GLYPH_STORIES__ && window.__GLYPH_STORIES__.palette) || { background: "#1d1f21", foreground: "#c5c8c6", cursor: "#c5c8c6", colors: {} },
      live: !!(window.__GLYPH_STORIES__ && window.__GLYPH_STORIES__.live),
      connected: false,
      busy: false,
      query: "",
      mode: "plain",
      showGolden: false,
      si: 0,
      ni: 0,
      selectedKey: "",
      modes: [
        { id: "plain", label: "plain" },
        { id: "grid", label: "grid" },
        { id: "spaces", label: "spaces" },
        { id: "diff", label: "diff" },
      ],
      inspect: { xy: "—", ch: "—", fg: "—", bg: "—", st: "—" },

      init() {
        if (this.live) this.connect();
        this.$watch("si", () => {
          const s = this.stories[this.si];
          this.selectedKey = s ? s.key : "";
          this.showGolden = false;
        });
        const first = this.stories[0];
        this.selectedKey = first ? first.key : "";
      },

      connect() {
        if (!window.EventSource) return;
        const es = new EventSource("/events");
        es.onopen = () => (this.connected = true);
        es.onerror = () => (this.connected = false);
        es.addEventListener("catalog", (ev) => {
          try {
            const next = JSON.parse(ev.data);
            this.apply(next);
          } catch (e) {
            /* ignore malformed frames */
          }
        });
        es.addEventListener("busy", (ev) => {
          this.busy = ev.data === "true";
        });
      },

      apply(next) {
        this.payload = next;
        this.stories = next.stories || [];
        this.summary = next.summary || {};
        if (next.palette) this.palette = next.palette;
        const idx = this.stories.findIndex((s) => s.key === this.selectedKey);
        this.si = idx >= 0 ? idx : 0;
        const cur = this.stories[this.si];
        if (cur && this.ni >= (cur.snapshots || []).length) this.ni = 0;
      },

      rerun(key) {
        if (!this.live) return;
        this.busy = true;
        fetch("/run", { method: "POST", headers: { "content-type": "application/json" }, body: JSON.stringify({ key: key || "" }) })
          .catch(() => {})
          .finally(() => (this.busy = false));
      },

      accept(key) {
        if (!this.live || !key) return;
        this.busy = true;
        fetch("/update", { method: "POST", headers: { "content-type": "application/json" }, body: JSON.stringify({ key }) })
          .catch(() => {})
          .finally(() => (this.busy = false));
      },

      label(s) {
        if (s.id) return s.variant ? s.id + " @" + s.variant : s.id;
        return s.name;
      },

      changedCount(s) {
        return (s.snapshots || []).reduce((n, p) => n + (p.golden === "changed" ? p.changed : 0), 0);
      },

      dotClass(s) {
        if (s.status === "passed" && s.golden !== "changed") return "bg-[#27c93f]";
        if (s.status === "not_run") return "bg-ink-line";
        if (s.golden === "changed" || s.status === "failed") return "bg-[#ff5f56]";
        return "bg-[#ffbd2e]";
      },

      get summaryLine() {
        const s = this.summary || {};
        const bits = [(s.stories || 0) + " stories"];
        if (s.passed) bits.push(s.passed + " passed");
        if (s.failed) bits.push(s.failed + " failed");
        if (s.changed) bits.push(s.changed + " changed");
        if (s.missing) bits.push(s.missing + " no golden");
        if (s.notRun) bits.push(s.notRun + " not run");
        return bits.join(" · ");
      },

      get groups() {
        const q = this.query.trim().toLowerCase();
        const g = {};
        this.stories.forEach((s, i) => {
          if (q && !(this.label(s) + " " + s.name + " " + (s.feature || "") + " " + (s.status || "")).toLowerCase().includes(q)) return;
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

      get hasDiff() {
        const p = this.snap;
        return !!(p && p.diff && p.diff.length);
      },

      get svg() {
        const p = this.snap;
        if (!p || p.status !== "ok") return "";
        const mode = this.mode === "diff" && !this.hasDiff ? "plain" : this.mode;
        const screen = { cols: p.cols, rows: p.rows, cells: cellsOf(p), cursor: p.cursor };
        const opts = {};
        if (mode === "grid") {
          opts.grid = true;
          opts.rulers = true;
          opts.regions = p.regions || [];
        } else if (mode === "spaces") {
          opts.grid = true;
          opts.rulers = true;
          opts.spaces = true;
          opts.regions = p.regions || [];
        } else if (mode === "diff") {
          opts.grid = true;
          opts.rulers = true;
          opts.diff = p.diff || [];
          if (this.showGolden && goldenCellsOf(p)) {
            screen.cells = goldenCellsOf(p);
            screen.cursor = null;
          }
        }
        return renderSVG(screen, opts, this.palette);
      },

      get windowTitle() {
        const s = this.current;
        if (!s) return "";
        return this.label(s) + " · " + s.name + (s.runId ? " · " + s.runId : "");
      },

      get size() {
        const p = this.snap;
        if (!p) return "";
        return (p.cols || 0) + "×" + (p.rows || 0);
      },

      get emptyMsg() {
        const s = this.current;
        if (!s) return "select a story";
        if (s.parseError) return "parse error: " + s.parseError;
        const p = this.snap;
        if (!p) {
          if (s.status === "not_run") return "not run — glyph stories run" + (s.id ? " --only " + s.id : "");
          return s.status + (s.diagnostic ? " — " + s.diagnostic : "");
        }
        if (p.status !== "ok") return "unreadable snapshot " + (p.error || "");
        return "";
      },

      get inspectRows() {
        const s = this.current;
        const golden = this.snap ? this.snap.golden : "—";
        return [
          { k: "cell", v: this.inspect.xy },
          { k: "char", v: this.inspect.ch },
          { k: "fg / bg", v: this.inspect.fg + " / " + this.inspect.bg },
          { k: "style", v: this.inspect.st },
          { k: "golden", v: golden + (s && s.diagnostic && s.status !== "passed" ? " · " + s.diagnostic : "") },
          { k: "keys", v: "j k stories · [ ] snaps · 1-4 mode · g golden" + (this.live ? " · r rerun · a accept" : "") },
        ];
      },

      select(i, n) {
        this.si = i;
        this.ni = n || 0;
      },

      origin() {
        const rulers = this.mode !== "plain";
        return { x: GEO.pad + (rulers ? GEO.rulerX : 0), y: GEO.pad + (rulers ? GEO.rulerY : 0) };
      },

      hover(ev) {
        const svg = ev.currentTarget.querySelector("svg");
        if (!svg || !this.snap) return;
        const r = svg.getBoundingClientRect();
        const vb = svg.viewBox.baseVal;
        const o = this.origin();
        const x = Math.floor(((ev.clientX - r.left) * (vb.width / r.width) - o.x) / GEO.cw);
        const y = Math.floor(((ev.clientY - r.top) * (vb.height / r.height) - o.y) / GEO.ch);
        const cells = this.mode === "diff" && this.showGolden && goldenCellsOf(this.snap) ? goldenCellsOf(this.snap) : cellsOf(this.snap);
        const cell = (cells || []).find((c) => c.x === x && c.y === y);
        const raw = cell && cell.char ? cell.char : " ";
        const shown = raw === " " ? "·" : raw;
        const st = cell && cell.style ? cell.style : {};
        const attrs =
          [st.bold && "bold", st.dim && "dim", st.italic && "italic", st.underline && "underline", st.reverse && "reverse"].filter(Boolean).join(" ") || "none";
        let diffNote = "";
        if (this.snap.diff) {
          const d = this.snap.diff.find((c) => c.x === x && c.y === y);
          if (d) diffNote = " · was " + JSON.stringify(d.before.char || " ") + " now " + JSON.stringify(d.after.char || " ");
        }
        this.inspect = {
          xy: x + "," + y,
          ch: JSON.stringify(shown) + diffNote,
          fg: st.fg || "default",
          bg: st.bg || "default",
          st: attrs + (cell && cell.link ? " · " + cell.link : ""),
        };
      },

      onKey(ev) {
        if (ev.target && ev.target.tagName === "INPUT") return;
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
        if (ev.key === "4" && this.hasDiff) this.mode = "diff";
        if (ev.key === "g" && this.hasDiff) {
          this.mode = "diff";
          this.showGolden = !this.showGolden;
        }
        if (ev.key === "r" && this.live) this.rerun(this.current && this.current.key);
        if (ev.key === "a" && this.live && this.current) this.accept(this.current.key);
        if (ev.key === "/") {
          const input = document.querySelector('input[type="search"]');
          if (input) {
            input.focus();
            ev.preventDefault();
          }
        }
      },
    }));
  });
})();
