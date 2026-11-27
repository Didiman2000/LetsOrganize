// SPDX-License-Identifier: GPL-3.0-or-later
//
// Oberfläche des Ringbuchs: Register umschalten, Seiten füllen, umblättern.
// Texte kommen aus /api/messages/<sprache>, Datumsangaben über Intl in der
// gewählten Sprache – so funktionieren auch andere Ziffern und Kalendernamen.

(() => {
  "use strict";

  const binder = document.getElementById("binder");
  const pageStart = document.getElementById("page-start");
  const pageEnd = document.getElementById("page-end");
  const tabs = Array.from(document.querySelectorAll(".tab"));
  const lang = binder.dataset.lang;
  const weekStart = Number(binder.dataset.weekStart || 1);
  const rtl = document.documentElement.dir === "rtl";

  let messages = {};
  let section = "calendar";
  let weekAnchor = startOfWeek(new Date());

  // Auswahl von Sprache und Thema sofort übernehmen.
  document.querySelectorAll("[data-autosubmit]").forEach((el) =>
    el.addEventListener("change", () => el.form.submit())
  );

  const t = (key, vars = {}) =>
    (messages[key] || key).replace(/\{(\w+)\}/g, (_, k) => (k in vars ? vars[k] : `{${k}}`));

  const el = (tag, attrs = {}, ...children) => {
    const node = document.createElement(tag);
    for (const [k, v] of Object.entries(attrs)) {
      if (k === "class") node.className = v;
      else if (k.startsWith("on")) node.addEventListener(k.slice(2), v);
      else node.setAttribute(k, v);
    }
    for (const c of children) node.append(c);
    return node;
  };

  const fmt = (opts) => new Intl.DateTimeFormat(lang, opts);
  const fmtNum = new Intl.NumberFormat(lang);

  function startOfWeek(d) {
    const x = new Date(d.getFullYear(), d.getMonth(), d.getDate());
    const diff = (x.getDay() - weekStart + 7) % 7;
    x.setDate(x.getDate() - diff);
    return x;
  }

  function addDays(d, n) {
    const x = new Date(d);
    x.setDate(x.getDate() + n);
    return x;
  }

  // Kalenderwoche nach ISO 8601.
  function isoWeek(d) {
    const x = new Date(Date.UTC(d.getFullYear(), d.getMonth(), d.getDate()));
    const day = x.getUTCDay() || 7;
    x.setUTCDate(x.getUTCDate() + 4 - day);
    const yearStart = new Date(Date.UTC(x.getUTCFullYear(), 0, 1));
    return Math.ceil(((x - yearStart) / 86400000 + 1) / 7);
  }

  const sameDay = (a, b) =>
    a.getFullYear() === b.getFullYear() && a.getMonth() === b.getMonth() && a.getDate() === b.getDate();

  function setPaper(paper) {
    for (const p of [pageStart, pageEnd]) {
      p.className = p.className.replace(/\bpaper-\S+/g, "").trim();
      p.classList.add(`paper-${paper}`);
    }
  }

  function turn() {
    for (const p of [pageStart, pageEnd]) {
      p.classList.remove("turning");
      void p.offsetWidth; // Animation neu starten
      p.classList.add("turning");
    }
  }

  // ---------- Abschnitte ----------

  function dayBlock(d) {
    const today = sameDay(d, new Date());
    const dow = d.getDay();
    const weekend = dow === (weekStart + 5) % 7 || dow === (weekStart + 6) % 7;
    return el(
      "div",
      { class: `day${today ? " today" : ""}${weekend ? " weekend" : ""}` },
      el(
        "div",
        { class: "day-head" },
        el("span", { class: "day-num" }, fmt({ day: "numeric" }).format(d)),
        el("span", { class: "day-name" }, fmt({ weekday: "long" }).format(d))
      ),
      el("div", { class: "day-empty" }, t("calendar.empty"))
    );
  }

  function renderCalendar() {
    const days = Array.from({ length: 7 }, (_, i) => addDays(weekAnchor, i));
    const monthTitle = fmt({ month: "long", year: "numeric" }).format(days[0]);
    const week = t("calendar.week", { n: fmtNum.format(isoWeek(days[3])) });

    const nav = el(
      "div",
      { class: "nav" },
      el("button", { type: "button", "aria-label": t("calendar.prev"), title: t("calendar.prev"),
        onclick: () => { weekAnchor = addDays(weekAnchor, -7); render(true); } }, rtl ? "›" : "‹"),
      el("button", { type: "button", onclick: () => { weekAnchor = startOfWeek(new Date()); render(true); } }, t("calendar.today")),
      el("button", { type: "button", "aria-label": t("calendar.next"), title: t("calendar.next"),
        onclick: () => { weekAnchor = addDays(weekAnchor, 7); render(true); } }, rtl ? "‹" : "›")
    );

    pageStart.replaceChildren(
      el("header", { class: "page-head" }, el("h1", {}, monthTitle), el("span", { class: "meta" }, week)),
      el("div", { class: "days" }, ...days.slice(0, 3).map(dayBlock))
    );
    pageEnd.replaceChildren(
      el("header", { class: "page-head" }, el("h2", {}, t("tab.calendar")), nav),
      el("div", { class: "days" }, ...days.slice(3).map(dayBlock))
    );
  }

  function renderPlanner() {
    const year = weekAnchor.getFullYear();
    const grid = (from, to) => {
      const g = el("div", { class: "planner" }, el("span"));
      for (let d = 1; d <= 31; d++) g.append(el("span", { class: "h" }, d % 5 === 0 || d === 1 ? fmtNum.format(d) : ""));
      for (let m = from; m < to; m++) {
        g.append(el("span", { class: "m" }, fmt({ month: "short" }).format(new Date(year, m, 1))));
        const len = new Date(year, m + 1, 0).getDate();
        for (let d = 1; d <= 31; d++) {
          if (d > len) { g.append(el("span", { class: "d x" })); continue; }
          const dow = new Date(year, m, d).getDay();
          const we = dow === (weekStart + 5) % 7 || dow === (weekStart + 6) % 7;
          g.append(el("span", { class: `d${we ? " we" : ""}`, title: fmt({ dateStyle: "full" }).format(new Date(year, m, d)) }));
        }
      }
      return g;
    };
    const yearText = fmt({ year: "numeric" }).format(new Date(year, 0, 1));
    pageStart.replaceChildren(
      el("header", { class: "page-head" }, el("h1", {}, t("tab.planner")), el("span", { class: "meta" }, yearText)),
      grid(0, 6)
    );
    pageEnd.replaceChildren(
      el("header", { class: "page-head" }, el("h2", {}, yearText)),
      grid(6, 12)
    );
  }

  function renderAddresses() {
    // Buchstabenleiste vorerst lateinisch; je Sprache einstellbar in Phase 3.
    const letters = "ABCDEFGHIJKLMNOPQRSTUVWXYZ".split("");
    pageStart.replaceChildren(
      el("header", { class: "page-head" }, el("h1", {}, t("tab.addresses"))),
      el("div", { class: "az" }, ...letters.map((l) => el("span", {}, l)))
    );
    pageEnd.replaceChildren(el("p", { class: "soon" }, t("section.soon")));
  }

  function renderGeneric() {
    pageStart.replaceChildren(el("header", { class: "page-head" }, el("h1", {}, t(`tab.${section}`))));
    pageEnd.replaceChildren(el("p", { class: "soon" }, t("section.soon")));
  }

  function render(animate) {
    binder.dataset.section = section;
    const tab = tabs.find((b) => b.dataset.tab === section);
    tabs.forEach((b) => b.setAttribute("aria-current", String(b === tab)));
    setPaper(tab ? tab.dataset.paper : "lined");
    ({ calendar: renderCalendar, planner: renderPlanner, addresses: renderAddresses }[section] || renderGeneric)();
    if (animate) turn();
  }

  tabs.forEach((b) =>
    b.addEventListener("click", () => {
      if (section === b.dataset.tab) return;
      section = b.dataset.tab;
      render(true);
    })
  );

  // Bild-auf/-ab blättert in der Kalenderwoche.
  document.addEventListener("keydown", (e) => {
    if (section !== "calendar" || e.target.closest("input, textarea, select")) return;
    if (e.key === "PageDown") { weekAnchor = addDays(weekAnchor, 7); render(true); }
    if (e.key === "PageUp") { weekAnchor = addDays(weekAnchor, -7); render(true); }
  });

  fetch(`/api/messages/${encodeURIComponent(lang)}`)
    .then((r) => r.json())
    .then((data) => { messages = data.messages || {}; })
    .catch(() => {})
    .finally(() => render(false));
})();
