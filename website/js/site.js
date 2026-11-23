/* htmlclay.com — Zero-dependency progressive enhancement */
(function () {
  "use strict";

  var DL_BASE = "https://download.htmlclay.com/";
  var reducedMotionQuery = window.matchMedia
    ? window.matchMedia("(prefers-reduced-motion: reduce)")
    : { matches: false };

  function detectOS() {
    var ua = navigator.userAgent || "";
    if (/Mac/.test(ua) && !/iPhone|iPad|iPod/.test(ua)) return "mac";
    if (/Win/.test(ua)) return "windows";
    if (/Linux|Android|CrOS/.test(ua)) return "linux";
    return "other";
  }

  /* 1. OS-Aware Download Button & /download/ Hub Highlighting */
  function initOSDownloads() {
    var os = detectOS();
    var macLink = document.querySelector("[data-mac-dmg]");
    var macUrl = macLink
      ? macLink.getAttribute("href")
      : DL_BASE + "HTMLClay-1.16.4-universal.dmg";

    var primary = {
      mac: {
        url: macUrl,
        label: "Download for macOS",
        note: "Universal DMG (Apple Silicon + Intel)",
        trust: "Notarized by Apple"
      },
      windows: {
        url: DL_BASE + "htmlclay-windows-amd64.zip",
        label: "Download for Windows",
        note: "64-bit portable ZIP",
        trust: "Signed for Windows"
      },
      linux: {
        url: DL_BASE + "htmlclay-linux-amd64.tar.gz",
        label: "Download for Linux",
        note: "x86_64 tarball"
      }
    };

    var btn = document.getElementById("download-btn");
    var label = document.getElementById("download-label");
    var sub = document.getElementById("download-sub");
    var trustLabel = document.getElementById("platform-trust-label");

    if (btn && label) {
      var fallbackHref = btn.getAttribute("data-fallback-href") || "#downloads";
      if (primary[os]) {
        btn.href = primary[os].url;
        btn.setAttribute("download", "");
        btn.dataset.hvOs = os;
        label.textContent = primary[os].label;
        if (sub) {
          sub.innerHTML =
            primary[os].note +
            ' &middot; <a href="/download/">all platforms &amp; checksums</a>';
        }
        if (trustLabel && primary[os].trust) {
          trustLabel.textContent = primary[os].trust;
        }
      } else {
        btn.href = fallbackHref;
        btn.removeAttribute("download");
        label.textContent = "Download HTML Clay";
        if (sub) {
          sub.innerHTML = '<a href="/download/">All platforms &amp; SHA-256 checksums</a>';
        }
      }
    }

    var osCards = document.querySelectorAll("[data-os-card]");
    if (osCards.length && primary[os]) {
      for (var i = 0; i < osCards.length; i++) {
        var card = osCards[i];
        var match = card.getAttribute("data-os-card") === os;
        card.classList.toggle("is-detected", match);
      }
    }
    var macCardBtn = document.querySelector("[data-mac-card-btn]");
    if (macCardBtn && macUrl) {
      macCardBtn.setAttribute("href", macUrl);
    }
  }

  /* 2. Roving Tabindex Helper */
  function attachRovingTabindex(tablist, tabs, onActivate) {
    if (!tablist || !tabs.length) return;
    tablist.addEventListener("keydown", function (e) {
      var idx = tabs.indexOf(document.activeElement);
      if (idx < 0) return;
      var next = -1;
      if (e.key === "ArrowRight" || e.key === "ArrowDown") {
        next = (idx + 1) % tabs.length;
      } else if (e.key === "ArrowLeft" || e.key === "ArrowUp") {
        next = (idx - 1 + tabs.length) % tabs.length;
      } else if (e.key === "Home") {
        next = 0;
      } else if (e.key === "End") {
        next = tabs.length - 1;
      }
      if (next >= 0) {
        e.preventDefault();
        tabs[next].focus();
        onActivate(tabs[next]);
      }
    });
  }

  /* 3. Interactive 7-Pane Settings Explorer */
  var PANE_ICONS = {
    general:
      '<svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"><path d="M10.7 2.2L13.3 2.2L13.7 4.3A7.9 7.9 0 0 1 16.24 5.35L18.01 4.15L19.85 5.99L18.65 7.76A7.9 7.9 0 0 1 19.7 10.3L21.8 10.7L21.8 13.3L19.7 13.7A7.9 7.9 0 0 1 18.65 16.24L19.85 18.01L18.01 19.85L16.24 18.65A7.9 7.9 0 0 1 13.7 19.7L13.3 21.8L10.7 21.8L10.3 19.7A7.9 7.9 0 0 1 7.76 18.65L5.99 19.85L4.15 18.01L5.35 16.24A7.9 7.9 0 0 1 4.3 13.7L2.2 13.3L2.2 10.7L4.3 10.3A7.9 7.9 0 0 1 5.35 7.76L4.15 5.99L5.99 4.15L7.76 5.35A7.9 7.9 0 0 1 10.3 4.3Z"/><circle cx="12" cy="12" r="3.2"/></svg>',
    ai:
      '<svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"><path d="M9.9 3.2 12 8.5l5.3 2.1L12 12.7 9.9 18l-2.1-5.3L2.5 10.6l5.3-2.1Z"/><path d="m18.5 14.5.9 2.1 2.1.9-2.1.9-.9 2.1-.9-2.1-2.1-.9 2.1-.9Z"/></svg>',
    folders:
      '<svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"><path d="M20 20a2 2 0 0 0 2-2V8a2 2 0 0 0-2-2h-7.9a2 2 0 0 1-1.69-.9L9.6 3.9A2 2 0 0 0 7.93 3H4a2 2 0 0 0-2 2v13a2 2 0 0 0 2 2Z"/></svg>',
    programs:
      '<svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"><rect x="2" y="4" width="20" height="16" rx="2"/><path d="m6 9 4 3-4 3"/><path d="M13 15h5"/></svg>',
    sync:
      '<svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"><path d="M21 12a9 9 0 0 0-9-9 9.75 9.75 0 0 0-6.74 2.74L3 8"/><path d="M3 3v5h5"/><path d="M3 12a9 9 0 0 0 9 9 9.75 9.75 0 0 0 6.74-2.74L21 16"/><path d="M16 16h5v5"/></svg>',
    updates:
      '<svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="9"/><path d="M12 8v8"/><path d="m8.5 12.5 3.5 3.5 3.5-3.5"/></svg>',
    advanced:
      '<svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"><line x1="4" y1="6" x2="20" y2="6"/><line x1="4" y1="12" x2="20" y2="12"/><line x1="4" y1="18" x2="20" y2="18"/><circle cx="9" cy="6" r="2" fill="currentColor"/><circle cx="15" cy="12" r="2" fill="currentColor"/><circle cx="10" cy="18" r="2" fill="currentColor"/></svg>'
  };

  var PANE_DATA = {
    general: {
      label: "General",
      title: "General: Startup, .htmlclay file opening & appearance",
      desc: "Open the local dashboard at htmlclay.localhost:1212, choose whether HTML Clay starts at login, and manage double-click opening for .htmlclay files while keeping your regular .html browser untouched (with HTML Clay ready in Open With). Set your optional document author name and System, Light, or Dark appearance.",
      alt: "HTML Clay General settings pane showing local dashboard link, Welcome window button, Open at login toggle, .htmlclay file association toggle, optional author name, and System/Light/Dark appearance.",
      badgesHtml:
        '<span class="filechip">htmlclay.localhost:1212</span><span class="filechip">.htmlclay &amp; Open With .html</span>'
    },
    ai: {
      label: "AI Editing",
      title: "AI Editing: Modify any element in place with Claude, Codex, or Gemini",
      desc: "Press \u2318J inside an HTML file to reshape selected text or layout using an AI CLI installed on your computer. Pick your preferred tool, model, and reasoning effort across Claude, Codex, or Gemini (agy), or wire up any custom command. Only your selected HTML and prompt are sent.",
      alt: "HTML Clay AI Editing settings pane showing Enable AI editing toggle and model plus effort pickers for Claude, Codex, Gemini (agy), and a custom CLI command.",
      badgesHtml:
        '<span class="kiln-pill kiln-pill-amber">\u2318J inline AI</span><span class="filechip">Claude \u00b7 Codex \u00b7 Gemini</span>'
    },
    folders: {
      label: "Folders",
      title: "Folders: Three clear levels of trust, local serving, and live sync",
      desc: "Start with a single folder on your computer and step up only when a project needs more: Trusted lets files save without prompts, Served gives a project folder its own local web address so pages share assets and link together, and Synced mirrors chosen folders live with Hyperclay.",
      alt: "HTML Clay Folders settings pane showing local Notes and prototypes folders alongside synced Hyperclay team folders with Trusted, Served, and Synced pills plus Open, Finder, and More controls.",
      badgesHtml:
        '<span class="kiln-pill kiln-pill-trusted">Trusted</span>' +
        '<span class="kiln-pill kiln-pill-served">Served</span>' +
        '<span class="kiln-pill kiln-pill-synced">Synced</span>'
    },
    programs: {
      label: "Programs",
      title: "Programs: Explicit approval for local helper scripts",
      desc: "Documents can ask HTML Clay to run a local script or binary on your computer, and every request waits for your answer. Review pending requests in one place, try built-in file search or image conversion examples, see which documents may run each program, or revoke access anytime.",
      alt: "HTML Clay Programs settings pane showing a waiting request from todo.htmlclay for todo-search and per-document access rules.",
      badgesHtml:
        '<span class="kiln-pill kiln-pill-amber">Per-document consent</span><span class="filechip">Pinned binary identity</span>'
    },
    sync: {
      label: "Sync",
      title: "Sync: Optional live two-way folder sync with Hyperclay",
      desc: "Local .htmlclay files never require an account. Whenever you want cloud backups, live web hosting, or team folders, connect your Hyperclay sync key, choose which folders mirror, pause syncing in one click, or settle two-way edits in the dashboard.",
      alt: "HTML Clay Sync settings pane showing connected Hyperclay account, pause syncing control, and per-folder sync status.",
      badgesHtml:
        '<span class="kiln-pill kiln-pill-synced">Live Hyperclay sync</span>'
    },
    updates: {
      label: "Updates",
      title: "Updates: Signed background updates that keep your tabs open",
      desc: "HTML Clay checks a cryptographically signed feed and installs updates automatically through Sparkle on macOS (and verified self-updates on Windows and Linux), or lets you check by hand whenever you prefer. Restarting finishes active saves first and reopens your tabs right where you left off.",
      alt: "HTML Clay Updates settings pane showing current version status, Check now button, and Update automatically toggle.",
      badgesHtml:
        '<span class="kiln-pill kiln-pill-synced">Signed update feed</span>'
    },
    advanced: {
      label: "Advanced",
      title: "Advanced: Local port, version storage & plain-text logs",
      desc: "See or change the single local port behind your htmlclay.localhost addresses (1212 by default), inspect the folder service status, reveal stored document versions and uploads directly in Finder, or open the app and sync logs when troubleshooting.",
      alt: "HTML Clay Advanced settings pane showing port 1212, running folder service, Finder buttons for versions and uploads storage, and log viewers.",
      badgesHtml:
        '<span class="filechip">Port 1212</span><span class="filechip">Show in Finder</span>'
    }
  };

  var PANE_ORDER = [
    "general",
    "ai",
    "folders",
    "programs",
    "sync",
    "updates",
    "advanced"
  ];

  var preloadedUrls = {};
  var SETTINGS_IMG_VER = "ce7e4f0e";
  function preloadImage(url) {
    if (!url || preloadedUrls[url]) return;
    preloadedUrls[url] = true;
    var img = new Image();
    img.src = url;
  }

  function paneImageUrl(pane, theme) {
    return "/images/v2/settings-" + pane + "-" + theme + ".webp?v=" + SETTINGS_IMG_VER;
  }

  function initSettingsExplorers() {
    var explorers = document.querySelectorAll("[data-settings-explorer]");
    for (var i = 0; i < explorers.length; i++) {
      setupExplorer(explorers[i]);
    }
  }

  function setupExplorer(root) {
    var currentPane = root.getAttribute("data-default-pane") || "folders";
    var currentTheme = root.getAttribute("data-theme") || "light";

    var tablist = root.querySelector('[role="tablist"]');
    var tabs = [].slice.call(root.querySelectorAll("[data-pane]"));
    var prevBtn = root.querySelector("[data-pane-prev]");
    var nextBtn = root.querySelector("[data-pane-next]");
    var nameEl = root.querySelector("[data-pane-name]");
    var iconEl = root.querySelector("[data-pane-icon]");
    var hotspots = [].slice.call(root.querySelectorAll("[data-pane-hotspot]"));
    var themeBtns = [].slice.call(
      root.querySelectorAll("[data-explorer-theme] [data-theme-val]")
    );
    var img = root.querySelector("[data-settings-img]");
    var panel = root.querySelector("[data-settings-panel]");
    var titleEl = root.querySelector("[data-pane-title]");
    var descEl = root.querySelector("[data-pane-desc]");
    var badgesEl = root.querySelector("[data-pane-badges]");

    function stepBy(delta) {
      var idx = PANE_ORDER.indexOf(currentPane);
      if (idx < 0) idx = 0;
      currentPane = PANE_ORDER[(idx + delta + PANE_ORDER.length) % PANE_ORDER.length];
      render();
    }

    function preloadNeighbors(pane, theme) {
      var idx = PANE_ORDER.indexOf(pane);
      if (idx < 0) return;
      var prev = PANE_ORDER[(idx - 1 + PANE_ORDER.length) % PANE_ORDER.length];
      var next = PANE_ORDER[(idx + 1) % PANE_ORDER.length];
      preloadImage(paneImageUrl(prev, theme));
      preloadImage(paneImageUrl(next, theme));
      preloadImage(paneImageUrl(pane, theme === "light" ? "dark" : "light"));
    }

    function render() {
      var info = PANE_DATA[currentPane] || PANE_DATA.folders;
      root.setAttribute("data-theme", currentTheme);

      tabs.forEach(function (tab) {
        var active = tab.getAttribute("data-pane") === currentPane;
        tab.setAttribute("aria-selected", String(active));
        tab.setAttribute("tabindex", active ? "0" : "-1");
        if (active && panel && tab.id) {
          panel.setAttribute("aria-labelledby", tab.id);
        }
      });

      if (nameEl) nameEl.textContent = info.label || currentPane;
      if (iconEl && PANE_ICONS[currentPane]) {
        iconEl.innerHTML = PANE_ICONS[currentPane];
      }

      themeBtns.forEach(function (b) {
        var on = b.getAttribute("data-theme-val") === currentTheme;
        b.setAttribute("aria-pressed", String(on));
      });

      if (img) {
        var nextSrc = paneImageUrl(currentPane, currentTheme);
        if (img.getAttribute("src") !== nextSrc) {
          img.setAttribute("src", nextSrc);
        }
        img.setAttribute("alt", info.alt);
      }

      if (titleEl) titleEl.textContent = info.title;
      if (descEl) descEl.textContent = info.desc;
      if (badgesEl) badgesEl.innerHTML = info.badgesHtml;

      preloadNeighbors(currentPane, currentTheme);
    }

    var stepper = root.querySelector(".explorer-stepper");
    if (prevBtn) {
      prevBtn.addEventListener("click", function () {
        stepBy(-1);
      });
    }
    if (nextBtn) {
      nextBtn.addEventListener("click", function () {
        stepBy(1);
      });
    }
    if (stepper) {
      stepper.addEventListener("keydown", function (e) {
        if (e.key === "ArrowLeft") {
          e.preventDefault();
          stepBy(-1);
        } else if (e.key === "ArrowRight") {
          e.preventDefault();
          stepBy(1);
        }
      });
    }

    tabs.forEach(function (tab) {
      var p = tab.getAttribute("data-pane");
      tab.addEventListener("click", function () {
        currentPane = p;
        render();
      });
      tab.addEventListener("mouseenter", function () {
        preloadImage(paneImageUrl(p, currentTheme));
      });
      tab.addEventListener("focus", function () {
        preloadImage(paneImageUrl(p, currentTheme));
      });
    });

    hotspots.forEach(function (hs) {
      var p = hs.getAttribute("data-pane-hotspot");
      hs.addEventListener("click", function () {
        currentPane = p;
        render();
      });
      hs.addEventListener("mouseenter", function () {
        preloadImage(paneImageUrl(p, currentTheme));
      });
    });

    themeBtns.forEach(function (b) {
      b.addEventListener("click", function () {
        currentTheme = b.getAttribute("data-theme-val") || "light";
        render();
      });
    });

    attachRovingTabindex(tablist, tabs, function (tab) {
      currentPane = tab.getAttribute("data-pane");
      render();
    });

    render();
  }

  /* 4. Standalone Light / Dark UI Stages (Hero Stage + Sunday Dinner + Popover, Security Approval Dialog) */
  function initThemeStages() {
    var stages = document.querySelectorAll("[data-theme-stage]");
    for (var i = 0; i < stages.length; i++) {
      (function (stage) {
        var btns = [].slice.call(stage.querySelectorAll("[data-stage-theme-btn]"));
        var imgs = [].slice.call(stage.querySelectorAll("img[data-light-src][data-dark-src]"));

        function setTheme(theme) {
          stage.setAttribute("data-theme", theme);
          btns.forEach(function (b) {
            b.setAttribute(
              "aria-pressed",
              String(b.getAttribute("data-stage-theme-btn") === theme)
            );
          });
          imgs.forEach(function (im) {
            var target =
              theme === "dark"
                ? im.getAttribute("data-dark-src")
                : im.getAttribute("data-light-src");
            if (target && im.getAttribute("src") !== target) {
              im.setAttribute("src", target);
            }
          });
        }

        btns.forEach(function (b) {
          b.addEventListener("click", function () {
            setTheme(b.getAttribute("data-stage-theme-btn") || "light");
          });
          b.addEventListener("mouseenter", function () {
            imgs.forEach(function (im) {
              preloadImage(im.getAttribute("data-dark-src"));
            });
          });
        });
      })(stages[i]);
    }
  }

  /* 5. Copy SHA-256 Checksum Buttons */
  function initCopyHashButtons() {
    var live = document.getElementById("copy-status-live");
    if (!live) {
      live = document.createElement("div");
      live.id = "copy-status-live";
      live.className = "sr-only";
      live.setAttribute("aria-live", "polite");
      document.body.appendChild(live);
    }

    var buttons = document.querySelectorAll("[data-copy-sha]");
    for (var i = 0; i < buttons.length; i++) {
      (function (btn) {
        var timer = 0;
        btn.addEventListener("click", function () {
          var targetFile = btn.getAttribute("data-copy-sha");
          var wrap = btn.closest(".dl-row") || document;
          var codeEl = targetFile
            ? wrap.querySelector('[data-sha256="' + targetFile + '"]') ||
              document.querySelector('[data-sha256="' + targetFile + '"]')
            : wrap.querySelector("[data-sha256]");
          if (!codeEl) return;
          var hash = (codeEl.textContent || "").trim();
          if (!hash) return;

          function onCopied() {
            btn.textContent = "Copied";
            btn.classList.add("is-copied");
            live.textContent =
              "Copied SHA-256 checksum for " + (targetFile || "download");
            if (timer) clearTimeout(timer);
            timer = setTimeout(function () {
              btn.textContent = "Copy SHA-256";
              btn.classList.remove("is-copied");
            }, 2000);
          }

          if (navigator.clipboard && navigator.clipboard.writeText) {
            navigator.clipboard.writeText(hash).then(onCopied, function () {
              fallbackCopy(hash, onCopied);
            });
          } else {
            fallbackCopy(hash, onCopied);
          }
        });
      })(buttons[i]);
    }
  }

  function fallbackCopy(text, cb) {
    var ta = document.createElement("textarea");
    ta.value = text;
    ta.setAttribute("readonly", "");
    ta.style.position = "fixed";
    ta.style.opacity = "0";
    document.body.appendChild(ta);
    ta.select();
    try {
      document.execCommand("copy");
      cb();
    } catch (e) {}
    document.body.removeChild(ta);
  }

  /* 6. 8-App sap.min.js Playground Switcher (Roving Tabindex + Auto-Select) */
  function initPlayground() {
    var chipBar = document.getElementById("app-chips");
    if (!chipBar) return;
    var chips = [].slice.call(chipBar.querySelectorAll(".chip[data-app]"));
    var panels = [].slice.call(
      document.querySelectorAll("#app-preview .app-panel[data-app]")
    );
    if (!chips.length || !panels.length) return;

    function autoSelect(p) {
      if (!p || p.__did || !p.hasAttribute("data-autoselect")) return;
      p.__did = true;
      var first = p.querySelector(".pick");
      if (first) first.click();
    }

    function show(app) {
      chips.forEach(function (c) {
        var active = c.dataset.app === app;
        c.setAttribute("aria-selected", String(active));
        c.setAttribute("tabindex", active ? "0" : "-1");
      });
      panels.forEach(function (p) {
        var on = p.dataset.app === app;
        if (on) {
          if (p.hidden) {
            p.hidden = false;
            p.dispatchEvent(new CustomEvent("app:show"));
          }
          autoSelect(p);
        } else if (!p.hidden) {
          p.hidden = true;
          p.dispatchEvent(new CustomEvent("app:hide"));
        }
      });
    }

    chips.forEach(function (c) {
      c.addEventListener("click", function () {
        show(c.dataset.app);
      });
    });

    attachRovingTabindex(chipBar, chips, function (chip) {
      show(chip.dataset.app);
    });

    document
      .querySelectorAll("input[type=date][data-default-today]")
      .forEach(function (el) {
        el.value = new Date().toISOString().slice(0, 10);
      });

    show("wiki");
  }

  /* 7. Breakout Canvas Game (Kiln Warm Palette, Zero Pure White/Black, Reduced Motion Safe) */
  function initBreakout() {
    var el = document.querySelector('.app-panel[data-app="game"]');
    if (!el || el.dataset.bkInit === "1") return;
    el.dataset.bkInit = "1";

    var canvas = el.querySelector('[data-bk="canvas"]');
    var stage = el.querySelector(".bk-stage");
    var scoreOut = el.querySelector('[data-bk="score"]');
    var livesOut = el.querySelector('[data-bk="lives"]');
    if (!canvas || !stage) return;
    var ctx = canvas.getContext("2d");

    var THEME = {
      bg: "#FAF8F3",
      ink: "#1A1C23",
      muted: "#5F6373",
      line: "#EAE6DC",
      card: "#FCFBF7",
      shine: "#FAF8F3",
      accent: "#B8420E"
    };

    var ROW_COLORS = ["#B8420E", "#D1661B", "#EC9F2E", "#C8A055", "#AFA996"];
    var COLS = 8;
    var ROWS = 5;
    var START_LIVES = 3;
    var W = 480;
    var H = 340;
    var dpr = 1;

    var state = {
      bricks: [],
      paddle: { x: 0, y: 0, w: 84, h: 12, targetX: 0, squash: 0 },
      ball: { x: 0, y: 0, r: 6, vx: 0, vy: 0, stuck: true },
      particles: [],
      score: 0,
      lives: START_LIVES,
      flash: 0
    };

    var BRICK = { top: 36, gapX: 6, gapY: 7, sideMargin: 14, h: 16 };
    var running = false;
    var rafId = 0;
    var lastT = 0;
    var keyLeft = false;
    var keyRight = false;

    function isReducedMotion() {
      return Boolean(reducedMotionQuery && reducedMotionQuery.matches);
    }

    function rand(a, b) {
      return a + Math.random() * (b - a);
    }

    function brickLayout() {
      var usableW = W - BRICK.sideMargin * 2;
      var bw = (usableW - BRICK.gapX * (COLS - 1)) / COLS;
      return { bw: bw, bh: BRICK.h, x0: BRICK.sideMargin, y0: BRICK.top };
    }

    function buildBricks() {
      var lay = brickLayout();
      var list = [];
      for (var r = 0; r < ROWS; r++) {
        for (var c = 0; c < COLS; c++) {
          list.push({
            x: lay.x0 + c * (lay.bw + BRICK.gapX),
            y: lay.y0 + r * (lay.bh + BRICK.gapY),
            w: lay.bw,
            h: lay.bh,
            color: ROW_COLORS[r % ROW_COLORS.length],
            alive: true,
            appear: 1,
            row: r
          });
        }
      }
      return list;
    }

    function refillBricks() {
      state.bricks = buildBricks();
      if (!isReducedMotion()) {
        for (var i = 0; i < state.bricks.length; i++) {
          state.bricks[i].appear = -state.bricks[i].row * 0.05;
        }
      }
    }

    function resetBall(centerPaddle) {
      var p = state.paddle;
      if (centerPaddle) {
        p.targetX = W / 2;
        p.x = W / 2;
      }
      var b = state.ball;
      b.r = Math.max(5, Math.round(W * 0.013));
      b.stuck = true;
      b.x = p.x;
      b.y = p.y - p.h / 2 - b.r - 1;
      b.vx = 0;
      b.vy = 0;
    }

    function ballSpeed() {
      return W * 0.62;
    }

    function launchBall() {
      var b = state.ball;
      if (!b.stuck) return;
      b.stuck = false;
      var speed = ballSpeed();
      var ang = rand(-0.35, 0.35);
      b.vx = Math.sin(ang) * speed;
      b.vy = -Math.cos(ang) * speed;
      start();
    }

    function setupSizes() {
      var p = state.paddle;
      p.w = Math.max(64, Math.round(W * 0.18));
      p.h = Math.max(10, Math.round(H * 0.035));
      p.y = H - Math.round(H * 0.07);
      if (p.x === 0) {
        p.x = W / 2;
        p.targetX = W / 2;
      }
    }

    function resize() {
      var rect = stage.getBoundingClientRect();
      var innerW = rect.width - 20;
      if (innerW <= 0) innerW = 460;
      var cssW = innerW;
      var cssH = Math.round(cssW * (340 / 480));

      dpr = Math.max(1, Math.min(window.devicePixelRatio || 1, 3));
      canvas.style.width = cssW + "px";
      canvas.style.height = cssH + "px";
      canvas.width = Math.round(cssW * dpr);
      canvas.height = Math.round(cssH * dpr);

      var prevW = W;
      W = cssW;
      H = cssH;
      setupSizes();

      if (!state.bricks.length) {
        refillBricks();
        for (var j = 0; j < state.bricks.length; j++) state.bricks[j].appear = 1;
      } else {
        var lay = brickLayout();
        for (var i = 0; i < state.bricks.length; i++) {
          var bk = state.bricks[i];
          var col = i % COLS;
          var row = Math.floor(i / COLS);
          bk.x = lay.x0 + col * (lay.bw + BRICK.gapX);
          bk.y = lay.y0 + row * (lay.bh + BRICK.gapY);
          bk.w = lay.bw;
          bk.h = lay.bh;
        }
      }

      var ratio = prevW > 0 ? W / prevW : 1;
      state.paddle.x *= ratio;
      state.paddle.targetX *= ratio;
      if (state.ball.stuck) {
        resetBall(false);
      } else {
        state.ball.x *= ratio;
        state.ball.y *= ratio;
      }
      draw();
    }

    function pointerToX(clientX) {
      var rect = canvas.getBoundingClientRect();
      var x = ((clientX - rect.left) / rect.width) * W;
      return Math.max(state.paddle.w / 2, Math.min(W - state.paddle.w / 2, x));
    }

    canvas.addEventListener("mousemove", function (e) {
      state.paddle.targetX = pointerToX(e.clientX);
      if (isReducedMotion() && state.ball.stuck) {
        state.paddle.x = state.paddle.targetX;
        resetBall(false);
        draw();
      }
    });
    canvas.addEventListener(
      "touchmove",
      function (e) {
        if (e.touches && e.touches.length) {
          state.paddle.targetX = pointerToX(e.touches[0].clientX);
          e.preventDefault();
        }
      },
      { passive: false }
    );
    canvas.addEventListener(
      "touchstart",
      function (e) {
        if (e.touches && e.touches.length) {
          state.paddle.targetX = pointerToX(e.touches[0].clientX);
        }
        launchBall();
      },
      { passive: false }
    );
    canvas.addEventListener("click", launchBall);

    document.addEventListener("keydown", function (e) {
      if (!isVisible()) return;
      if (e.key === "ArrowLeft") {
        keyLeft = true;
        e.preventDefault();
        start();
      } else if (e.key === "ArrowRight") {
        keyRight = true;
        e.preventDefault();
        start();
      } else if (e.key === "ArrowUp" || e.key === " " || e.key === "Spacebar") {
        launchBall();
        e.preventDefault();
      }
    });
    document.addEventListener("keyup", function (e) {
      if (e.key === "ArrowLeft") keyLeft = false;
      else if (e.key === "ArrowRight") keyRight = false;
    });

    function spawnBurst(x, y, color) {
      if (isReducedMotion()) return;
      for (var i = 0; i < 7; i++) {
        var a = rand(0, Math.PI * 2);
        var sp = rand(40, 150);
        state.particles.push({
          x: x,
          y: y,
          vx: Math.cos(a) * sp,
          vy: Math.sin(a) * sp - 30,
          life: 1,
          decay: rand(1.8, 3.2),
          r: rand(1.5, 3),
          color: color
        });
      }
    }

    function renderHud() {
      if (scoreOut) scoreOut.textContent = String(state.score);
      if (livesOut) {
        var dots = livesOut.querySelectorAll(".bk-dot");
        if (dots.length !== START_LIVES) {
          livesOut.innerHTML = "";
          for (var i = 0; i < START_LIVES; i++) {
            var d = document.createElement("span");
            d.className = "bk-dot";
            livesOut.appendChild(d);
          }
          dots = livesOut.querySelectorAll(".bk-dot");
        }
        for (var j = 0; j < dots.length; j++) {
          dots[j].classList.toggle("is-lost", j >= state.lives);
        }
      }
    }

    function aliveCount() {
      var n = 0;
      for (var i = 0; i < state.bricks.length; i++) {
        if (state.bricks[i].alive) n++;
      }
      return n;
    }

    function loseLife() {
      state.lives -= 1;
      state.paddle.squash = isReducedMotion() ? 0 : 1;
      if (state.lives <= 0) {
        state.lives = START_LIVES;
        state.score = 0;
        refillBricks();
      }
      renderHud();
      resetBall(true);
    }

    function stepBall(dt) {
      var b = state.ball;
      var p = state.paddle;
      b.x += b.vx * dt;
      b.y += b.vy * dt;

      if (b.x - b.r < 0) {
        b.x = b.r;
        b.vx = Math.abs(b.vx);
      } else if (b.x + b.r > W) {
        b.x = W - b.r;
        b.vx = -Math.abs(b.vx);
      }
      if (b.y - b.r < 0) {
        b.y = b.r;
        b.vy = Math.abs(b.vy);
      }

      var pl = p.x - p.w / 2;
      var pr = p.x + p.w / 2;
      var pt = p.y - p.h / 2;
      if (
        b.vy > 0 &&
        b.y + b.r >= pt &&
        b.y - b.r <= p.y + p.h / 2 &&
        b.x >= pl - b.r &&
        b.x <= pr + b.r
      ) {
        b.y = pt - b.r;
        var hit = Math.max(-1, Math.min(1, (b.x - p.x) / (p.w / 2)));
        var ang = hit * 1.05;
        var sp = ballSpeed();
        b.vx = Math.sin(ang) * sp;
        b.vy = -Math.abs(Math.cos(ang) * sp);
        p.squash = isReducedMotion() ? 0 : 1;
      }

      for (var i = 0; i < state.bricks.length; i++) {
        var bk = state.bricks[i];
        if (!bk.alive) continue;
        if (
          b.x + b.r > bk.x &&
          b.x - b.r < bk.x + bk.w &&
          b.y + b.r > bk.y &&
          b.y - b.r < bk.y + bk.h
        ) {
          var overlapL = b.x + b.r - bk.x;
          var overlapR = bk.x + bk.w - (b.x - b.r);
          var overlapT = b.y + b.r - bk.y;
          var overlapB = bk.y + bk.h - (b.y - b.r);
          var minX = Math.min(overlapL, overlapR);
          var minY = Math.min(overlapT, overlapB);
          if (minX < minY) {
            b.vx = -b.vx;
            b.x += overlapL < overlapR ? -minX : minX;
          } else {
            b.vy = -b.vy;
            b.y += overlapT < overlapB ? -minY : minY;
          }
          bk.alive = false;
          state.score += 10;
          spawnBurst(bk.x + bk.w / 2, bk.y + bk.h / 2, bk.color);
          renderHud();
          break;
        }
      }

      if (b.y - b.r > H) {
        loseLife();
      }
    }

    function update(dt) {
      var p = state.paddle;
      var b = state.ball;
      var kSpeed = W * 1.15 * dt;
      if (keyLeft) p.targetX -= kSpeed;
      if (keyRight) p.targetX += kSpeed;
      p.targetX = Math.max(p.w / 2, Math.min(W - p.w / 2, p.targetX));

      if (isReducedMotion()) {
        p.x = p.targetX;
        p.squash = 0;
      } else {
        p.x += (p.targetX - p.x) * Math.min(1, dt * 18);
        if (p.squash > 0) p.squash = Math.max(0, p.squash - dt * 4);
      }

      if (b.stuck) {
        b.x = p.x;
        b.y = p.y - p.h / 2 - b.r - 1;
      } else {
        var steps = Math.max(1, Math.ceil((ballSpeed() * dt) / (b.r * 0.9)));
        var sdt = dt / steps;
        for (var s = 0; s < steps; s++) {
          stepBall(sdt);
          if (b.stuck) break;
        }
      }

      for (var i = state.particles.length - 1; i >= 0; i--) {
        var pt = state.particles[i];
        pt.life -= pt.decay * dt;
        if (pt.life <= 0) {
          state.particles.splice(i, 1);
          continue;
        }
        pt.vy += 220 * dt;
        pt.x += pt.vx * dt;
        pt.y += pt.vy * dt;
      }

      for (var k = 0; k < state.bricks.length; k++) {
        var bk = state.bricks[k];
        if (bk.appear < 1) bk.appear = Math.min(1, bk.appear + dt * 4.5);
      }

      if (state.flash > 0) state.flash = Math.max(0, state.flash - dt * 3.5);

      if (aliveCount() === 0) {
        refillBricks();
        state.flash = isReducedMotion() ? 0 : 0.45;
      }
    }

    function roundRect(c, x, y, w, h, r) {
      r = Math.min(r, w / 2, h / 2);
      c.beginPath();
      c.moveTo(x + r, y);
      c.arcTo(x + w, y, x + w, y + h, r);
      c.arcTo(x + w, y + h, x, y + h, r);
      c.arcTo(x, y + h, x, y, r);
      c.arcTo(x, y, x + w, y, r);
      c.closePath();
    }

    function draw() {
      ctx.setTransform(dpr, 0, 0, dpr, 0, 0);
      ctx.clearRect(0, 0, W, H);

      ctx.fillStyle = THEME.card;
      ctx.fillRect(0, 0, W, H);
      var grd = ctx.createLinearGradient(0, 0, 0, H);
      grd.addColorStop(0, "rgba(184, 66, 14, 0.04)");
      grd.addColorStop(0.4, "rgba(184, 66, 14, 0)");
      ctx.fillStyle = grd;
      ctx.fillRect(0, 0, W, H);

      if (state.flash > 0) {
        ctx.fillStyle = "rgba(236, 159, 46, " + (state.flash * 0.14).toFixed(3) + ")";
        ctx.fillRect(0, 0, W, H);
      }

      for (var i = 0; i < state.bricks.length; i++) {
        var bk = state.bricks[i];
        var ap = Math.max(0, Math.min(1, bk.appear));
        if (bk.alive && ap > 0) {
          var ease = 1 - Math.pow(1 - ap, 3);
          var cx = bk.x + bk.w / 2;
          var cy = bk.y + bk.h / 2;
          var sc = 0.7 + 0.3 * ease;
          var w = bk.w * sc;
          var h = bk.h * sc;
          ctx.globalAlpha = ease;
          ctx.fillStyle = bk.color;
          roundRect(ctx, cx - w / 2, cy - h / 2, w, h, 5);
          ctx.fill();
          ctx.globalAlpha = ease * 0.22;
          ctx.fillStyle = THEME.shine;
          roundRect(ctx, cx - w / 2 + 2, cy - h / 2 + 2, w - 4, h * 0.4, 4);
          ctx.fill();
          ctx.globalAlpha = 1;
        }
      }

      for (var p2 = 0; p2 < state.particles.length; p2++) {
        var pt = state.particles[p2];
        ctx.globalAlpha = Math.max(0, Math.min(1, pt.life));
        ctx.fillStyle = pt.color;
        ctx.beginPath();
        ctx.arc(pt.x, pt.y, pt.r, 0, Math.PI * 2);
        ctx.fill();
      }
      ctx.globalAlpha = 1;

      var p = state.paddle;
      var sq = p.squash;
      var pw = p.w * (1 + sq * 0.12);
      var ph = p.h * (1 - sq * 0.22);
      ctx.save();
      ctx.shadowColor = "rgba(184, 66, 14, 0.24)";
      ctx.shadowBlur = 10;
      ctx.shadowOffsetY = 2;
      ctx.fillStyle = THEME.accent;
      roundRect(ctx, p.x - pw / 2, p.y - ph / 2, pw, ph, ph / 2);
      ctx.fill();
      ctx.restore();
      ctx.globalAlpha = 0.24;
      ctx.fillStyle = THEME.shine;
      roundRect(ctx, p.x - pw / 2 + 3, p.y - ph / 2 + 2, pw - 6, ph * 0.4, ph * 0.3);
      ctx.fill();
      ctx.globalAlpha = 1;

      var b = state.ball;
      ctx.save();
      ctx.shadowColor = "rgba(26, 28, 35, 0.22)";
      ctx.shadowBlur = 8;
      ctx.shadowOffsetY = 2;
      ctx.fillStyle = THEME.ink;
      ctx.beginPath();
      ctx.arc(b.x, b.y, b.r, 0, Math.PI * 2);
      ctx.fill();
      ctx.restore();
      ctx.globalAlpha = 0.34;
      ctx.fillStyle = THEME.shine;
      ctx.beginPath();
      ctx.arc(b.x - b.r * 0.3, b.y - b.r * 0.3, b.r * 0.4, 0, Math.PI * 2);
      ctx.fill();
      ctx.globalAlpha = 1;

      if (b.stuck) {
        ctx.globalAlpha = isReducedMotion()
          ? 0.72
          : 0.48 + 0.26 * Math.sin(performance.now() / 340);
        ctx.fillStyle = THEME.muted;
        ctx.font = '600 12px ui-rounded, "SF Pro Rounded", system-ui, sans-serif';
        ctx.textAlign = "center";
        ctx.fillText("click, space, or \u2191 to launch", W / 2, p.y - 26);
        ctx.globalAlpha = 1;
      }
    }

    function frame(t) {
      if (!running) return;
      if (!lastT) lastT = t;
      var dt = Math.min(0.05, (t - lastT) / 1000);
      lastT = t;
      update(dt);
      draw();
      if (isReducedMotion() && state.ball.stuck && !keyLeft && !keyRight) {
        stop();
        return;
      }
      rafId = window.requestAnimationFrame(frame);
    }

    function start() {
      if (running || document.hidden) return;
      running = true;
      lastT = 0;
      rafId = window.requestAnimationFrame(frame);
    }

    function stop() {
      running = false;
      if (rafId) {
        window.cancelAnimationFrame(rafId);
        rafId = 0;
      }
    }

    function isVisible() {
      if (!el.isConnected) return false;
      var r = el.getBoundingClientRect();
      if (r.width === 0 && r.height === 0) return false;
      var cs = window.getComputedStyle(el);
      return cs.display !== "none" && cs.visibility !== "hidden";
    }

    el.addEventListener("app:show", function () {
      resize();
      for (var i = 0; i < state.bricks.length; i++) state.bricks[i].appear = 1;
      draw();
      if (!isReducedMotion()) start();
    });
    el.addEventListener("app:hide", stop);
    document.addEventListener("visibilitychange", function () {
      if (document.hidden) stop();
      else if (isVisible() && (!isReducedMotion() || !state.ball.stuck)) {
        lastT = 0;
        start();
      }
    });
    window.addEventListener("resize", resize);

    refillBricks();
    for (var m = 0; m < state.bricks.length; m++) state.bricks[m].appear = 1;
    resize();
    renderHud();
    draw();
    if (isVisible() && !document.hidden && !isReducedMotion()) start();
  }

  /* 8. Ownership 4x3 Visual Concept Switcher + Live Theme Swatches */
  function initOwnershipSwitchers() {
    var cards = document.querySelectorAll("[data-own-card]");
    for (var i = 0; i < cards.length; i++) {
      (function (card) {
        var slides = [].slice.call(card.querySelectorAll("[data-own-slide]"));
        var picks = [].slice.call(card.querySelectorAll("[data-own-pick]"));
        var prev = card.querySelector("[data-own-prev]");
        var next = card.querySelector("[data-own-next]");
        var caption = card.querySelector("[data-own-caption]");
        var tablist = card.querySelector('[role="tablist"]');
        if (!slides.length) return;
        var current = 0;

        function select(idx) {
          current = (idx + slides.length) % slides.length;
          slides.forEach(function (s, k) {
            s.hidden = k !== current;
          });
          picks.forEach(function (b, k) {
            var active = k === current;
            b.setAttribute("aria-selected", String(active));
            b.setAttribute("tabindex", active ? "0" : "-1");
          });
          if (caption && slides[current]) {
            caption.textContent = slides[current].getAttribute("data-label") || "";
          }
        }

        picks.forEach(function (b) {
          b.addEventListener("click", function () {
            select(Number(b.getAttribute("data-own-pick")) || 0);
          });
        });
        if (prev) {
          prev.addEventListener("click", function () {
            select(current - 1);
          });
        }
        if (next) {
          next.addEventListener("click", function () {
            select(current + 1);
          });
        }
        attachRovingTabindex(tablist, picks, function (btn) {
          select(Number(btn.getAttribute("data-own-pick")) || 0);
        });
      })(cards[i]);
    }

    var swatchRoot = document.querySelector("[data-swatch-demo]");
    if (swatchRoot) {
      var btns = [].slice.call(swatchRoot.querySelectorAll("[data-skin-pick]"));
      var skins = ["cozy", "ink", "botanical", "blueprint"];
      var skinIdx = 0;
      var userPinned = false;

      function applySkin(name) {
        var clean = (name || "cozy").trim();
        swatchRoot.setAttribute("data-skin", clean);
        skinIdx = Math.max(0, skins.indexOf(clean));
        btns.forEach(function (b) {
          var val = (b.getAttribute("data-skin-pick") || "").trim();
          b.setAttribute("aria-pressed", String(val === clean));
        });
      }

      btns.forEach(function (b) {
        b.addEventListener("click", function () {
          userPinned = true;
          applySkin(b.getAttribute("data-skin-pick"));
        });
      });

      if (!(reducedMotionQuery && reducedMotionQuery.matches)) {
        setInterval(function () {
          if (userPinned || swatchRoot.closest("[hidden]")) return;
          skinIdx = (skinIdx + 1) % skins.length;
          applySkin(skins[skinIdx]);
        }, 2400);
      }
    }
  }

  function boot() {
    initOSDownloads();
    initSettingsExplorers();
    initThemeStages();
    initCopyHashButtons();
    initPlayground();
    initBreakout();
    initOwnershipSwitchers();
  }

  if (document.readyState === "loading") {
    document.addEventListener("DOMContentLoaded", boot);
  } else {
    boot();
  }
})();
