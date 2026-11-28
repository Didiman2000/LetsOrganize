// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (C) 2026 Andreas "Nupela Didiman" Neugebauer
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

  // ---------- Termine ----------

  let events = [];
  let eventsLoaded = false;
  const dialog = document.getElementById("event-dialog");
  const form = dialog && dialog.querySelector("form");

  const pad = (n) => String(n).padStart(2, "0");
  const dateKey = (d) => `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}`;
  const utcKey = (d) => `${d.getUTCFullYear()}-${pad(d.getUTCMonth() + 1)}-${pad(d.getUTCDate())}`;
  const timeValue = (d) => `${pad(d.getHours())}:${pad(d.getMinutes())}`;

  async function api(method, url, body) {
    const opts = { method, headers: { "X-LetsOrganize": "1" } };
    if (body !== undefined) {
      opts.headers["Content-Type"] = "application/json";
      opts.body = JSON.stringify(body);
    }
    const res = await fetch(url, opts);
    if (!res.ok) {
      let msg = res.statusText;
      try { const j = await res.json(); msg = j.field ? `${j.field}: ${j.message}` : j.message || msg; } catch (_) {}
      throw new Error(msg);
    }
    return res.status === 204 ? null : res.json();
  }

  async function loadWeek() {
    const from = weekAnchor;
    const to = addDays(weekAnchor, 7);
    try {
      events = await api("GET", `/api/events?from=${encodeURIComponent(from.toISOString())}&to=${encodeURIComponent(to.toISOString())}`);
      eventsLoaded = true;
    } catch (_) {
      events = [];
      eventsLoaded = false;
    }
  }

  // Termine, die den Tag d (lokale Zeit) berühren.
  function eventsOn(d) {
    const key = dateKey(d);
    const dayStart = d;
    const dayEnd = addDays(d, 1);
    return events.filter((e) => {
      const s = new Date(e.start);
      const en = new Date(e.end);
      if (e.allDay) return utcKey(s) <= key && key < utcKey(en);
      return s < dayEnd && (en > dayStart || (+en === +s && s >= dayStart));
    });
  }

  function eventLabel(e, d) {
    if (e.allDay) return e.title;
    const s = new Date(e.start);
    const time = sameDay(s, d) ? fmt({ hour: "2-digit", minute: "2-digit" }).format(s) : "…";
    return `${time}  ${e.title}`;
  }

  function openDialog(e, day) {
    if (!dialog) return;
    form.reset();
    form.dataset.id = e ? e.id : "";
    dialog.querySelector(".dialog-title").textContent = t(e ? "event.edit" : "event.new");
    dialog.querySelector("[data-action=delete]").hidden = !e;
    dialog.querySelector(".dialog-error").textContent = "";
    const s = e ? new Date(e.start) : new Date(day.getFullYear(), day.getMonth(), day.getDate(), 9, 0);
    const en = e ? new Date(e.end) : new Date(s.getTime() + 3600000);
    form.elements.title.value = e ? e.title : "";
    form.elements.location.value = e ? e.location : "";
    form.elements.notes.value = e ? e.notes : "";
    form.elements.allDay.checked = e ? e.allDay : false;
    if (e && e.allDay) {
      form.elements.date.value = utcKey(s);
      form.elements.endDate.value = utcKey(new Date(en.getTime() - 86400000));
      form.elements.start.value = "09:00";
      form.elements.end.value = "10:00";
    } else {
      form.elements.date.value = dateKey(s);
      form.elements.endDate.value = dateKey(en);
      form.elements.start.value = timeValue(s);
      form.elements.end.value = timeValue(en);
    }
    syncAllDay();
    dialog.showModal();
    form.elements.title.focus();
  }

  function syncAllDay() {
    const allDay = form.elements.allDay.checked;
    form.querySelectorAll(".time-only").forEach((n) => (n.hidden = allDay));
  }

  function readForm() {
    const [y, m, d] = form.elements.date.value.split("-").map(Number);
    const [y2, m2, d2] = (form.elements.endDate.value || form.elements.date.value).split("-").map(Number);
    let start, end;
    if (form.elements.allDay.checked) {
      start = new Date(Date.UTC(y, m - 1, d));
      end = new Date(Date.UTC(y2, m2 - 1, d2 + 1));
    } else {
      const [sh, sm] = form.elements.start.value.split(":").map(Number);
      const [eh, em] = form.elements.end.value.split(":").map(Number);
      start = new Date(y, m - 1, d, sh, sm);
      end = new Date(y2, m2 - 1, d2, eh, em);
    }
    return {
      title: form.elements.title.value,
      location: form.elements.location.value,
      notes: form.elements.notes.value,
      allDay: form.elements.allDay.checked,
      start: start.toISOString(),
      end: end.toISOString(),
    };
  }

  if (form) {
    form.elements.allDay.addEventListener("change", syncAllDay);
    form.elements.date.addEventListener("change", () => {
      if (!form.elements.endDate.value || form.elements.endDate.value < form.elements.date.value) form.elements.endDate.value = form.elements.date.value;
    });
    form.addEventListener("submit", async (ev) => {
      if (ev.submitter && ev.submitter.value === "cancel") return;
      ev.preventDefault();
      const id = form.dataset.id;
      try {
        await api(id ? "PUT" : "POST", id ? `/api/events/${id}` : "/api/events", readForm());
        dialog.close();
        await loadWeek();
        render(false);
      } catch (err) {
        dialog.querySelector(".dialog-error").textContent = `${t("event.error")} ${err.message}`;
      }
    });
    dialog.querySelector("[data-action=delete]").addEventListener("click", async () => {
      const id = form.dataset.id;
      if (!id) return;
      try {
        await api("DELETE", `/api/events/${id}`);
        dialog.close();
        await loadWeek();
        render(false);
      } catch (err) {
        dialog.querySelector(".dialog-error").textContent = `${t("event.error")} ${err.message}`;
      }
    });
  }

  function dayBlock(d) {
    const today = sameDay(d, new Date());
    const dow = d.getDay();
    const weekend = dow === (weekStart + 5) % 7 || dow === (weekStart + 6) % 7;
    const list = eventsOn(d);
    const items = list.length
      ? list.map((e) =>
          el("button", { type: "button", class: `event${e.allDay ? " all-day" : ""}`, title: e.location || e.title,
            onclick: (ev) => { ev.stopPropagation(); openDialog(e); } }, eventLabel(e, d)))
      : [el("div", { class: "day-empty" }, t("calendar.empty"))];
    const block = el(
      "div",
      { class: `day${today ? " today" : ""}${weekend ? " weekend" : ""}` },
      el(
        "div",
        { class: "day-head" },
        el("span", { class: "day-num" }, fmt({ day: "numeric" }).format(d)),
        el("span", { class: "day-name" }, fmt({ weekday: "long" }).format(d)),
        eventsLoaded
          ? el("button", { type: "button", class: "day-add", "aria-label": t("event.new"), title: t("event.new"),
              onclick: (ev) => { ev.stopPropagation(); openDialog(null, d); } }, "+")
          : ""
      ),
      el("div", { class: "day-events" }, ...items)
    );
    if (eventsLoaded) block.addEventListener("dblclick", () => openDialog(null, d));
    return block;
  }

  function renderCalendar() {
    const days = Array.from({ length: 7 }, (_, i) => addDays(weekAnchor, i));
    const monthTitle = fmt({ month: "long", year: "numeric" }).format(days[0]);
    const week = t("calendar.week", { n: fmtNum.format(isoWeek(days[3])) });

    const nav = el(
      "div",
      { class: "nav" },
      el("button", { type: "button", "aria-label": t("calendar.prev"), title: t("calendar.prev"),
        onclick: () => goWeek(-7) }, rtl ? "›" : "‹"),
      el("button", { type: "button", onclick: () => goWeek(0) }, t("calendar.today")),
      el("button", { type: "button", "aria-label": t("calendar.next"), title: t("calendar.next"),
        onclick: () => goWeek(7) }, rtl ? "‹" : "›")
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

  async function goWeek(delta) {
    weekAnchor = delta === 0 ? startOfWeek(new Date()) : addDays(weekAnchor, delta);
    await loadWeek();
    render(true);
  }

  // ---------- Aufgaben ----------

  let tasksOpen = [];
  let tasksDone = [];
  let tasksLoaded = false;
  const taskDialog = document.getElementById("task-dialog");
  const taskForm = taskDialog && taskDialog.querySelector("form");

  async function loadTasks() {
    try {
      [tasksOpen, tasksDone] = await Promise.all([
        api("GET", "/api/tasks?status=open"),
        api("GET", "/api/tasks?status=done&limit=30"),
      ]);
      tasksLoaded = true;
    } catch (_) {
      tasksOpen = [];
      tasksDone = [];
      tasksLoaded = false;
    }
  }

  async function refreshTasks() {
    await loadTasks();
    if (section === "todo") render(false);
  }

  function dueInfo(due) {
    if (!due) return null;
    const [y, m, d] = due.split("-").map(Number);
    const date = new Date(y, m - 1, d);
    const today = new Date();
    const t0 = new Date(today.getFullYear(), today.getMonth(), today.getDate());
    const days = Math.round((date - t0) / 86400000);
    let cls = "";
    if (days < 0) cls = "overdue";
    else if (days === 0) cls = "due-today";
    const text = days === 0 ? t("calendar.today") : fmt({ day: "numeric", month: "short" }).format(date);
    return { cls, text, title: fmt({ dateStyle: "full" }).format(date) };
  }

  function taskRow(task) {
    const box = el("input", { type: "checkbox", "aria-label": t(task.done ? "task.reopen" : "task.complete") });
    box.checked = task.done;
    box.addEventListener("change", async () => {
      box.disabled = true;
      try {
        await api("PUT", `/api/tasks/${task.id}/done`, { done: box.checked });
      } catch (_) {
        box.checked = !box.checked;
      }
      refreshTasks();
    });
    const due = dueInfo(task.due);
    return el(
      "li",
      { class: `task${task.done ? " done" : ""} prio-${task.priority}` },
      box,
      el("button", { type: "button", class: "task-title", title: task.notes || task.title, onclick: () => openTaskDialog(task) },
        task.priority > 0 ? el("span", { class: "prio", "aria-label": t(`task.priority${task.priority}`) }, "!".repeat(task.priority)) : "",
        task.title),
      due ? el("span", { class: `due ${due.cls}`, title: due.title }, due.text) : ""
    );
  }

  function renderTodo() {
    const add = el("input", { class: "task-add", type: "text", maxlength: "500", placeholder: t("task.addPlaceholder"), "aria-label": t("task.new") });
    add.addEventListener("keydown", async (ev) => {
      if (ev.key !== "Enter" || !add.value.trim()) return;
      ev.preventDefault();
      const title = add.value;
      add.value = "";
      try {
        await api("POST", "/api/tasks", { title });
      } catch (_) {
        add.value = title;
      }
      await refreshTasks();
      const again = pageStart.querySelector(".task-add");
      if (again) again.focus();
    });
    const openList = tasksOpen.length
      ? el("ul", { class: "tasks" }, ...tasksOpen.map(taskRow))
      : el("p", { class: "soon" }, t(tasksLoaded ? "task.noneOpen" : "section.soon"));
    pageStart.replaceChildren(
      el("header", { class: "page-head" }, el("h1", {}, t("tab.todo")),
        el("span", { class: "meta" }, t("task.openCount", { n: fmtNum.format(tasksOpen.length) }))),
      tasksLoaded ? add : "",
      openList
    );
    pageEnd.replaceChildren(
      el("header", { class: "page-head" }, el("h2", {}, t("task.done"))),
      tasksDone.length ? el("ul", { class: "tasks" }, ...tasksDone.map(taskRow)) : el("p", { class: "soon" }, t("task.noneDone"))
    );
  }

  function openTaskDialog(task) {
    if (!taskDialog) return;
    taskForm.reset();
    taskForm.dataset.id = task ? task.id : "";
    taskDialog.querySelector(".dialog-title").textContent = t(task ? "task.edit" : "task.new");
    taskDialog.querySelector("[data-action=delete]").hidden = !task;
    taskDialog.querySelector(".dialog-error").textContent = "";
    const f = taskForm.elements;
    f.title.value = task ? task.title : "";
    f.due.value = task && task.due ? task.due : "";
    f.priority.value = String(task ? task.priority : 0);
    f.notes.value = task ? task.notes : "";
    f.done.checked = task ? task.done : false;
    taskForm._task = task;
    taskDialog.showModal();
    f.title.focus();
  }

  if (taskForm) {
    taskForm.addEventListener("submit", async (ev) => {
      if (ev.submitter && ev.submitter.value === "cancel") return;
      ev.preventDefault();
      const f = taskForm.elements;
      const id = taskForm.dataset.id;
      const old = taskForm._task;
      const body = {
        title: f.title.value,
        notes: f.notes.value,
        due: f.due.value || null,
        priority: Number(f.priority.value),
        done: f.done.checked,
        position: old ? old.position : 0,
      };
      try {
        await api(id ? "PUT" : "POST", id ? `/api/tasks/${id}` : "/api/tasks", body);
        taskDialog.close();
        refreshTasks();
      } catch (err) {
        taskDialog.querySelector(".dialog-error").textContent = `${t("event.error")} ${err.message}`;
      }
    });
    taskDialog.querySelector("[data-action=delete]").addEventListener("click", async () => {
      const id = taskForm.dataset.id;
      if (!id) return;
      try {
        await api("DELETE", `/api/tasks/${id}`);
        taskDialog.close();
        refreshTasks();
      } catch (err) {
        taskDialog.querySelector(".dialog-error").textContent = `${t("event.error")} ${err.message}`;
      }
    });
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

  // ---------- Adressen ----------

  const alphabet = Array.from(binder.dataset.alphabet || "ABCDEFGHIJKLMNOPQRSTUVWXYZ");
  let contacts = [];
  let contactsLoaded = false;
  let contactLetter = null;
  let contactSearch = "";
  let contactSelected = null;
  let searchTimer = 0;
  const contactDialog = document.getElementById("contact-dialog");
  const contactForm = contactDialog && contactDialog.querySelector("form");

  // Varianten wie أ, إ, آ stehen im Register unter ا.
  const letterAliases = { "أ": "ا", "إ": "ا", "آ": "ا", "ٱ": "ا", "ة": "ه", "ى": "ي", "ؤ": "و", "ئ": "ي" };
  const registerOf = (c) => {
    const l = letterAliases[c.letter] || c.letter;
    return alphabet.includes(l) ? l : "#";
  };

  async function loadContacts() {
    try {
      const q = contactSearch ? `?q=${encodeURIComponent(contactSearch)}` : "";
      contacts = await api("GET", `/api/contacts${q}`);
      contactsLoaded = true;
    } catch (_) {
      contacts = [];
      contactsLoaded = false;
    }
    if (contactSelected) contactSelected = contacts.find((c) => c.id === contactSelected.id) || null;
  }

  async function refreshContacts(keepFocus) {
    await loadContacts();
    if (section !== "addresses") return;
    render(false);
    if (keepFocus) {
      const box = pageStart.querySelector(".contact-search");
      if (box) { box.focus(); box.setSelectionRange(box.value.length, box.value.length); }
    }
  }

  const fieldText = (f) => {
    const kind = t(`contact.kind.${f.kind}`);
    return f.label ? `${kind} (${f.label})` : kind;
  };

  function fieldValue(f) {
    if (f.kind === "phone") return el("a", { href: `tel:${f.value.replace(/[^+0-9]/g, "")}` }, f.value);
    if (f.kind === "email") return el("a", { href: `mailto:${f.value}` }, f.value);
    if (f.kind === "url" && /^https?:\/\//i.test(f.value)) return el("a", { href: f.value, target: "_blank", rel: "noopener noreferrer" }, f.value);
    return el("span", { class: f.kind === "address" ? "multiline" : "" }, f.value);
  }

  function formatBirthday(b) {
    if (!b) return "";
    if (b.startsWith("--")) {
      const [m, d] = b.slice(2).split("-").map(Number);
      return fmt({ day: "numeric", month: "long" }).format(new Date(2000, m - 1, d));
    }
    const [y, m, d] = b.split("-").map(Number);
    return fmt({ dateStyle: "long" }).format(new Date(y, m - 1, d));
  }

  function contactCard(c) {
    const sub = [c.jobTitle, c.organization].filter(Boolean).join(", ");
    const rows = c.fields.map((f) => el("div", { class: "card-row" }, el("dt", {}, fieldText(f)), el("dd", {}, fieldValue(f))));
    if (c.birthday) rows.push(el("div", { class: "card-row" }, el("dt", {}, t("contact.birthday")), el("dd", {}, formatBirthday(c.birthday))));
    return el(
      "article",
      { class: "contact-card" },
      el("header", { class: "page-head" },
        el("h2", {}, c.displayName),
        el("div", { class: "nav" },
          el("button", { type: "button", onclick: () => openContactDialog(c) }, t("contact.editButton")),
          el("button", { type: "button", onclick: () => openContactDialog(null) }, "+"))),
      sub ? el("p", { class: "card-sub" }, sub) : "",
      el("dl", {}, ...rows),
      c.notes ? el("p", { class: "card-notes multiline" }, c.notes) : ""
    );
  }

  function renderAddresses() {
    const used = new Set(contacts.map(registerOf));
    if (!contactLetter || (!contactSearch && !used.has(contactLetter) && used.size)) {
      contactLetter = alphabet.find((l) => used.has(l)) || (used.has("#") ? "#" : alphabet[0]);
    }
    const letters = [...alphabet, "#"];
    const strip = el("div", { class: "az", role: "tablist" }, ...letters.map((l) =>
      el("button", {
        type: "button",
        class: `${used.has(l) ? "has" : ""}${!contactSearch && l === contactLetter ? " current" : ""}`,
        "aria-pressed": String(!contactSearch && l === contactLetter),
        onclick: () => { contactLetter = l; contactSearch = ""; refreshContacts(false); },
      }, l)));

    const search = el("input", { class: "contact-search", type: "search", maxlength: "200", placeholder: t("contact.search"), "aria-label": t("contact.search") });
    search.value = contactSearch;
    search.addEventListener("input", () => {
      clearTimeout(searchTimer);
      searchTimer = setTimeout(() => { contactSearch = search.value.trim(); refreshContacts(true); }, 250);
    });

    const shown = contactSearch ? contacts : contacts.filter((c) => registerOf(c) === contactLetter);
    const list = shown.length
      ? el("ul", { class: "contacts" }, ...shown.map((c) => {
          const phone = c.fields.find((f) => f.kind === "phone");
          return el("li", {},
            el("button", { type: "button", class: `contact-row${contactSelected && contactSelected.id === c.id ? " selected" : ""}`,
              onclick: () => { contactSelected = c; render(false); } },
              el("span", { class: "contact-name" }, c.displayName),
              el("span", { class: "contact-hint" }, c.organization && c.organization !== c.displayName ? c.organization : phone ? phone.value : "")));
        }))
      : el("p", { class: "soon" }, t(!contactsLoaded ? "section.soon" : contactSearch ? "contact.noResults" : "contact.none"));

    pageStart.replaceChildren(
      el("header", { class: "page-head" }, el("h1", {}, t("tab.addresses")),
        el("span", { class: "meta" }, t("contact.count", { n: fmtNum.format(contacts.length) }))),
      contactsLoaded ? search : "",
      strip,
      list
    );
    pageEnd.replaceChildren(
      contactSelected
        ? contactCard(contactSelected)
        : el("div", { class: "contact-empty" },
            el("p", { class: "soon" }, t("contact.pick")),
            contactsLoaded ? el("div", { class: "nav" }, el("button", { type: "button", onclick: () => openContactDialog(null) }, `+ ${t("contact.new")}`)) : "")
    );
  }

  function fieldRow(f) {
    const kind = el("select", { name: "fieldKind", "aria-label": t("contact.fields") },
      ...["phone", "email", "address", "url"].map((k) => el("option", { value: k }, t(`contact.kind.${k}`))));
    kind.value = f.kind;
    const label = el("input", { name: "fieldLabel", list: "contact-labels", maxlength: "100", placeholder: t("contact.label"), "aria-label": t("contact.label") });
    label.value = f.label || "";
    const value = el(f.kind === "address" ? "textarea" : "input", { name: "fieldValue", maxlength: "2000", rows: "2", placeholder: t("contact.value"), "aria-label": t("contact.value") });
    value.value = f.value || "";
    const row = el("div", { class: "field-row" }, kind, label, value,
      el("button", { type: "button", class: "remove", "aria-label": t("contact.remove"), title: t("contact.remove"), onclick: () => row.remove() }, "×"));
    kind.addEventListener("change", () => {
      const next = fieldRow({ kind: kind.value, label: label.value, value: row.querySelector("[name=fieldValue]").value });
      row.replaceWith(next);
    });
    return row;
  }

  function openContactDialog(c) {
    if (!contactDialog) return;
    contactForm.reset();
    contactForm.dataset.id = c ? c.id : "";
    contactDialog.querySelector(".dialog-title").textContent = t(c ? "contact.edit" : "contact.new");
    contactDialog.querySelector("[data-action=delete]").hidden = !c;
    contactDialog.querySelector(".dialog-error").textContent = "";
    const f = contactForm.elements;
    f.givenName.value = c ? c.givenName : "";
    f.familyName.value = c ? c.familyName : "";
    f.organization.value = c ? c.organization : "";
    f.jobTitle.value = c ? c.jobTitle : "";
    f.notes.value = c ? c.notes : "";
    const b = c && c.birthday;
    f.noYear.checked = !!(b && b.startsWith("--"));
    f.birthday.value = b ? (b.startsWith("--") ? `2000${b.slice(1)}` : b) : "";
    const box = contactDialog.querySelector(".fields");
    const fields = c && c.fields.length ? c.fields : [{ kind: "phone", label: "" }, { kind: "email", label: "" }];
    box.replaceChildren(...fields.map(fieldRow));
    contactDialog.showModal();
    f.givenName.focus();
  }

  if (contactForm) {
    contactDialog.querySelectorAll("[data-add-field]").forEach((btn) =>
      btn.addEventListener("click", () => {
        const row = fieldRow({ kind: btn.dataset.addField, label: "" });
        contactDialog.querySelector(".fields").append(row);
        row.querySelector("[name=fieldValue]").focus();
      }));
    contactForm.addEventListener("submit", async (ev) => {
      if (ev.submitter && ev.submitter.value === "cancel") return;
      ev.preventDefault();
      const f = contactForm.elements;
      let birthday = f.birthday.value || null;
      if (birthday && f.noYear.checked) birthday = `--${birthday.slice(5)}`;
      const fields = Array.from(contactDialog.querySelectorAll(".field-row")).map((row) => ({
        kind: row.querySelector("[name=fieldKind]").value,
        label: row.querySelector("[name=fieldLabel]").value,
        value: row.querySelector("[name=fieldValue]").value,
      }));
      const body = {
        givenName: f.givenName.value, familyName: f.familyName.value,
        organization: f.organization.value, jobTitle: f.jobTitle.value,
        birthday, notes: f.notes.value, fields,
      };
      const id = contactForm.dataset.id;
      try {
        const saved = await api(id ? "PUT" : "POST", id ? `/api/contacts/${id}` : "/api/contacts", body);
        contactDialog.close();
        contactSelected = saved;
        contactLetter = registerOf(saved);
        contactSearch = "";
        refreshContacts(false);
      } catch (err) {
        contactDialog.querySelector(".dialog-error").textContent = `${t("event.error")} ${err.message}`;
      }
    });
    contactDialog.querySelector("[data-action=delete]").addEventListener("click", async () => {
      const id = contactForm.dataset.id;
      if (!id) return;
      try {
        await api("DELETE", `/api/contacts/${id}`);
        contactDialog.close();
        contactSelected = null;
        refreshContacts(false);
      } catch (err) {
        contactDialog.querySelector(".dialog-error").textContent = `${t("event.error")} ${err.message}`;
      }
    });
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
    ({ calendar: renderCalendar, todo: renderTodo, planner: renderPlanner, addresses: renderAddresses }[section] || renderGeneric)();
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
    if (document.querySelector("dialog[open]")) return;
    if (e.key === "PageDown") goWeek(7);
    if (e.key === "PageUp") goWeek(-7);
  });

  fetch(`/api/messages/${encodeURIComponent(lang)}`)
    .then((r) => r.json())
    .then((data) => { messages = data.messages || {}; })
    .catch(() => {})
    .then(() => Promise.all([loadWeek(), loadTasks(), loadContacts()]))
    .finally(() => render(false));
})();
