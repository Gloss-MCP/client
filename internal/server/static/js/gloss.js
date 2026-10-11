// gloss.js -- small hand-written Alpine.js helpers for the annotation
// UI. Not vendored (it's app code, same status as app.css): served
// same-origin, no build step, no CDN.

// glossCodeView backs the code pane's line selection and the
// gutter highlighting of already-annotated lines. It operates
// generically on [data-line] elements -- the join key any
// line-anchoring plugin's rendered HTML is expected to carry
// (internal/plugins/lines.go) -- rather than knowing anything about how
// a given plugin rendered them. Elements covering several lines (a
// markdown block, a csv record) also carry data-line-end; csv cells
// carry data-col, which turns a click into a cell selection.
function glossCodeView(threadLines) {
  return {
    selStart: null,
    selEnd: null,
    // selCol is the 1-based column when a single csv cell is selected
    // (elements carrying data-col), else null for a plain line range.
    selCol: null,
    threadLines: threadLines || [],

    // span returns the [start, end] source lines an element covers.
    // Line-per-element plugins carry only data-line; block-per-element
    // plugins (markdown's rendered view) also carry data-line-end.
    span(el) {
      const start = parseInt(el.dataset.line, 10);
      const end = el.dataset.lineEnd ? parseInt(el.dataset.lineEnd, 10) : start;
      return [start, end];
    },

    init() {
      this.$el.querySelectorAll('[data-line]').forEach((el) => {
        const [start, end] = this.span(el);
        if (this.threadLines.some((n) => n >= start && n <= end)) {
          el.classList.add('bg-amber-50');
        }
      });
      this.$watch('selStart', () => this.paintSelection());
      this.$watch('selEnd', () => this.paintSelection());
      this.$watch('selCol', () => this.paintSelection());
    },

    onLineClick(evt) {
      const el = evt.target.closest('[data-line]');
      if (!el) return;
      const [start, end] = this.span(el);
      if (evt.shiftKey && this.selStart !== null) {
        this.selEnd = start >= this.selStart ? end : start;
        this.selCol = null; // a multi-row range is no longer one cell
      } else {
        this.selStart = start;
        this.selEnd = end;
        const cell = evt.target.closest('[data-col]');
        this.selCol = cell && el.contains(cell) ? parseInt(cell.dataset.col, 10) : null;
      }
    },

    selRange() {
      if (this.selStart === null) return { start: null, end: null };
      return {
        start: Math.min(this.selStart, this.selEnd),
        end: Math.max(this.selStart, this.selEnd),
      };
    },

    selLabel() {
      const r = this.selRange();
      if (r.start === null) return '';
      const lines = r.start === r.end ? String(r.start) : r.start + '-' + r.end;
      return this.selCol === null ? lines : lines + ', column ' + this.selCol;
    },

    clearSelection() {
      this.selStart = null;
      this.selEnd = null;
      this.selCol = null;
    },

    paintSelection() {
      const r = this.selRange();
      this.$el.querySelectorAll('[data-line]').forEach((el) => {
        const [start, end] = this.span(el);
        el.classList.toggle('bg-blue-50', r.start !== null && start <= r.end && end >= r.start);
      });
      this.$el.querySelectorAll('[data-col]').forEach((cell) => {
        const row = cell.closest('[data-line]');
        const picked = this.selCol !== null && row !== null &&
          parseInt(row.dataset.line, 10) === r.start &&
          parseInt(cell.dataset.col, 10) === this.selCol;
        cell.classList.toggle('gloss-cell-selected', picked);
      });
    },
  };
}

// glossRegionView backs the selection and highlighting for a media
// surface -- an element carrying [data-region-surface] around an <img>
// (internal/plugins/image.go). Dragging on the image draws a rectangle,
// stored as fractions (0-1) of the displayed image so it is independent of
// zoom and window size; the composer submits it as x/y/width/height.
// Existing region threads arrive as `marks` (JSON from the server) and are
// drawn as numbered boxes on an overlay layer; orphaned threads draw no
// box (they stay visible in the thread panel).
//
// The line-based glossCodeView is deliberately untouched: it knows
// nothing about regions, and this knows nothing about lines.
function glossRegionView(marks) {
  // DOM handles and in-flight drag state live in the closure, not on the
  // returned object, so Alpine's reactivity doesn't proxy them.
  let layer = null;
  let origin = null; // {x, y} fractions where the drag began
  let live = null; // rectangle being dragged, painted but not yet committed

  const clamp = (v) => Math.min(1, Math.max(0, v));
  const pct = (f) => (f * 100) + '%';

  return {
    marks: marks || [],
    // sel is the committed selection {x, y, width, height}, or null.
    sel: null,
    // MIN_SIZE is the smallest side (fraction) a drag must span to count
    // as a selection rather than a stray click.
    MIN_SIZE: 0.01,
    ready: false,

    init() {
      if (this.ready) return;
      this.ready = true;
      const surface = this.$el.querySelector('[data-region-surface]');
      if (!surface) return;
      layer = document.createElement('div');
      layer.className = 'gloss-region-layer';
      surface.appendChild(layer);

      surface.addEventListener('pointerdown', (e) => this.onPointerDown(e, surface));
      surface.addEventListener('pointermove', (e) => this.onPointerMove(e, surface));
      surface.addEventListener('pointerup', (e) => this.onPointerUp(e, surface));
      surface.addEventListener('pointercancel', () => this.cancelDrag());
      window.addEventListener('keydown', (e) => {
        if (e.key === 'Escape') this.clearSelection();
      });
      this.$watch('sel', () => this.paint());
      this.paint();
    },

    // point converts a pointer event to fractions of the displayed image.
    point(e, surface) {
      const el = surface.querySelector('img, video');
      const r = (el || surface).getBoundingClientRect();
      if (r.width === 0 || r.height === 0) return null;
      return { x: clamp((e.clientX - r.left) / r.width), y: clamp((e.clientY - r.top) / r.height) };
    },

    rectBetween(a, b) {
      return {
        x: Math.min(a.x, b.x),
        y: Math.min(a.y, b.y),
        width: Math.abs(a.x - b.x),
        height: Math.abs(a.y - b.y),
      };
    },

    onPointerDown(e, surface) {
      if (e.button !== 0 || e.target.closest('.gloss-mark')) return;
      const p = this.point(e, surface);
      if (!p) return;
      origin = p;
      live = this.rectBetween(p, p);
      surface.setPointerCapture(e.pointerId);
      e.preventDefault(); // no native image drag / text selection
      this.paint();
    },

    onPointerMove(e, surface) {
      if (!origin) return;
      const p = this.point(e, surface);
      if (!p) return;
      live = this.rectBetween(origin, p);
      this.paint();
    },

    onPointerUp(e, surface) {
      if (!origin) return;
      const p = this.point(e, surface);
      const r = p ? this.rectBetween(origin, p) : live;
      this.cancelDrag();
      if (r && r.width >= this.MIN_SIZE && r.height >= this.MIN_SIZE) this.sel = r;
    },

    cancelDrag() {
      origin = null;
      live = null;
      this.paint();
    },

    clearSelection() {
      this.sel = null;
    },

    selLabel() {
      const s = this.sel;
      if (!s) return '';
      const p = (f) => Math.round(f * 100) + '%';
      return p(s.x) + ',' + p(s.y) + ' ' + p(s.width) + '×' + p(s.height);
    },

    // field formats one selection coordinate for the composer's hidden
    // inputs: empty until there is a selection.
    field(name) {
      return this.sel ? this.sel[name].toFixed(6) : '';
    },

    focusThread(id) {
      const el = document.getElementById('thread-' + id);
      if (!el) return;
      el.scrollIntoView({ block: 'nearest' });
      el.classList.add('gloss-thread-flash');
      setTimeout(() => el.classList.remove('gloss-thread-flash'), 1200);
    },

    // shown reports whether mark m is drawn right now. Orphaned threads
    // draw no box; images draw only region threads (glossVideoView adds a
    // time check).
    shown(m) {
      return m.type === 'region' && m.status !== 'orphaned';
    },

    place(el, r) {
      el.style.left = pct(r.x);
      el.style.top = pct(r.y);
      el.style.width = pct(r.width);
      el.style.height = pct(r.height);
    },

    paint() {
      if (!layer) return;
      layer.replaceChildren();
      for (const m of this.marks) {
        if (!this.shown(m)) continue;
        const box = document.createElement('button');
        box.type = 'button';
        box.className = 'gloss-mark' + (m.status === 'resolved' ? ' gloss-mark-resolved' : '');
        box.dataset.thread = m.id;
        box.textContent = String(m.n);
        box.setAttribute('aria-label', 'Thread ' + m.n);
        this.place(box, m);
        box.addEventListener('click', (e) => {
          e.stopPropagation();
          this.focusThread(m.id);
        });
        layer.appendChild(box);
      }
      const r = live || this.sel;
      if (r) {
        const box = document.createElement('div');
        box.className = 'gloss-region-sel';
        this.place(box, r);
        layer.appendChild(box);
      }
    },
  };
}

// glossTimeView backs time-range selection on an audio surface
// (internal/plugins/audio.go): play the media, "Mark start" / "Mark end"
// capture currentTime, and the composer submits start_time / end_time in
// seconds. Existing time threads arrive as `marks` and render as chips
// that seek the player to the range start and focus the thread.
function glossTimeView(marks) {
  const two = (n) => String(Math.floor(n)).padStart(2, '0');
  const clock = (t) => Math.floor(t / 60) + ':' + two(t % 60) + '.' + Math.floor((t % 1) * 10);

  return {
    marks: (marks || []).filter((m) => m.type === 'time'),
    startT: null,
    endT: null,
    // media is the player element; looked up per call so the helpers
    // below can be reused by glossVideoView.
    media() {
      return this.$root.querySelector('audio, video');
    },

    now() {
      const m = this.media();
      return m ? m.currentTime : 0;
    },

    markStart() {
      this.startT = this.now();
      if (this.endT !== null && this.endT < this.startT) this.endT = null;
    },

    markEnd() {
      if (this.startT === null) return;
      this.endT = Math.max(this.now(), this.startT);
    },

    clearSelection() {
      this.startT = null;
      this.endT = null;
    },

    // An unset end means an instant: end = start.
    timeField(which) {
      if (this.startT === null) return '';
      const t = which === 'start' ? this.startT : (this.endT ?? this.startT);
      return t.toFixed(3);
    },

    rangeLabel(start, end) {
      return start === end ? clock(start) : clock(start) + '\u2013' + clock(end);
    },

    selLabel() {
      if (this.startT === null) return '';
      return this.rangeLabel(this.startT, this.endT ?? this.startT);
    },

    // Orphaned threads draw no chip (they stay in the thread panel).
    visibleMarks() {
      return this.marks.filter((m) => m.status !== 'orphaned');
    },

    seekTo(m) {
      const player = this.media();
      if (player) {
        player.pause();
        player.currentTime = m.start;
      }
      const el = document.getElementById('thread-' + m.id);
      if (!el) return;
      el.scrollIntoView({ block: 'nearest' });
      el.classList.add('gloss-thread-flash');
      setTimeout(() => el.classList.remove('gloss-thread-flash'), 1200);
    },
  };
}

// glossVideoView combines glossRegionView (draw a rectangle over the
// video frame) and glossTimeView (mark a time range) for region-time
// anchors. Dragging is only live in "draw" mode, so it never fights the
// native player controls: drawing pauses the video and takes the current
// time as the range start; "Mark end" extends it. A region_time thread's
// box is drawn only while the playhead is inside its time range.
function glossVideoView(marks) {
  const base = glossRegionView(marks);
  const time = glossTimeView(marks);
  const TOLERANCE = 0.25; // seconds either side that a mark stays visible

  return {
    ...base,
    ...time,
    marks: (marks || []).filter((m) => m.type === 'region_time'),
    drawing: false,
    startT: null,
    endT: null,

    init() {
      base.init.call(this);
      const video = this.media();
      if (video) {
        for (const ev of ['timeupdate', 'seeked', 'loadedmetadata']) {
          video.addEventListener(ev, () => this.paint());
        }
      }
      const surface = this.$el.querySelector('[data-region-surface]');
      this.$watch('drawing', (on) => surface && surface.classList.toggle('gloss-drawing', on));
    },

    toggleDrawing() {
      this.drawing = !this.drawing;
      if (this.drawing) {
        const video = this.media();
        if (video) video.pause();
      }
    },

    shown(m) {
      if (m.type !== 'region_time' || m.status === 'orphaned') return false;
      const t = this.now();
      return t >= m.start - TOLERANCE && t <= m.end + TOLERANCE;
    },

    onPointerDown(e, surface) {
      if (!this.drawing) return;
      this.startT = this.now();
      this.endT = null;
      base.onPointerDown.call(this, e, surface);
    },

    onPointerUp(e, surface) {
      base.onPointerUp.call(this, e, surface);
      if (this.sel) this.drawing = false; // back to the player controls
    },

    markEnd() {
      if (this.sel === null) return;
      this.endT = Math.max(this.now(), this.startT);
    },

    clearSelection() {
      this.sel = null;
      this.startT = null;
      this.endT = null;
    },

    selLabel() {
      if (!this.sel) return '';
      return base.selLabel.call(this) + ' at ' + this.rangeLabel(this.startT, this.endT ?? this.startT);
    },
  };
}
