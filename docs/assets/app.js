(function () {
  "use strict";

  var root = document.documentElement;
  var API = "https://api.github.com/repos/ilyasaftr/OpenPrintCast/releases/latest";
  var CACHE_KEY = "opc-latest-release";
  var CACHE_MS = 10 * 60 * 1000;

  /* ---- Download links: resolve the latest release in the browser. ---- */

  // The Windows processor of this visitor: arm64 when the browser says ARM.
  function detectArch() {
    var data = navigator.userAgentData;
    if (data && data.getHighEntropyValues) {
      return data.getHighEntropyValues(["architecture"]).then(function (v) {
        return /arm/i.test(v.architecture || "") ? "arm64" : "amd64";
      }, function () { return "amd64"; });
    }
    return Promise.resolve(/arm64|aarch64/i.test(navigator.userAgent) ? "arm64" : "amd64");
  }

  function latestRelease() {
    try {
      var cached = JSON.parse(sessionStorage.getItem(CACHE_KEY) || "null");
      if (cached && Date.now() - cached.at < CACHE_MS) return Promise.resolve(cached.release);
    } catch (e) { /* storage unavailable */ }
    return fetch(API, { headers: { Accept: "application/vnd.github+json" } })
      .then(function (r) { if (!r.ok) throw new Error(String(r.status)); return r.json(); })
      .then(function (release) {
        try { sessionStorage.setItem(CACHE_KEY, JSON.stringify({ at: Date.now(), release: release })); } catch (e) { /* ignore */ }
        return release;
      });
  }

  function assetUrl(release, kind, arch) {
    var want = kind === "installer"
      ? new RegExp("^OpenPrintCast-Setup-.+-" + arch + "\\.exe$")
      : new RegExp("^OpenPrintCast-" + arch + "\\.exe$");
    var hit = (release.assets || []).filter(function (a) { return want.test(a.name); })[0];
    return hit ? hit.browser_download_url : "";
  }

  Promise.all([latestRelease().catch(function () { return null; }), detectArch()]).then(function (res) {
    var release = res[0], arch = res[1];
    var suggested = document.querySelector('.dl[data-arch="' + arch + '"]');
    if (suggested) suggested.classList.add("suggested");
    if (!release) return;

    var version = document.getElementById("version");
    if (version && release.tag_name) version.textContent = "Version " + release.tag_name.replace(/^v/, "");

    document.querySelectorAll("a[data-asset]").forEach(function (a) {
      var url = assetUrl(release, a.getAttribute("data-asset"), a.getAttribute("data-arch"));
      if (url) a.href = url;
    });

    var cta = document.getElementById("cta-download");
    var installer = assetUrl(release, "installer", arch);
    if (cta && installer) {
      cta.href = installer;
      cta.setAttribute("aria-label", "Download the installer for Windows (" + arch + ")");
    }
  });

  /* ---- Motion (GSAP). Everything is visible without it. ---- */

  if (root.classList.contains("no-gsap") || !window.gsap || !window.ScrollTrigger) {
    root.classList.add("no-gsap");
    return;
  }

  try {
    animate();
  } catch (err) {
    // Never leave content hidden because an animation failed.
    root.classList.add("no-gsap");
    if (window.console) console.error(err);
  }

  function animate() {
    var gsap = window.gsap, ScrollTrigger = window.ScrollTrigger;
    gsap.registerPlugin(ScrollTrigger);
    gsap.config({ nullTargetWarn: false });
    ScrollTrigger.config({ ignoreMobileResize: true });

    var mm = gsap.matchMedia();

    // Visitors who ask for reduced motion get the finished page (CSS shows it).
    mm.add("(prefers-reduced-motion: no-preference)", function () {
      intro();
      scrollReveals();
      screenshots();
      navigation();
      faq();
    });

    // Pointer effects only for a mouse or trackpad, never for touch.
    mm.add("(hover: hover) and (pointer: fine) and (prefers-reduced-motion: no-preference)", function () {
      magnetic(".btn-large, .nav .btn-primary");
      depth(".hero-shot");
    });

    window.addEventListener("load", function () { ScrollTrigger.refresh(); });
  }

  // The headline arrives word by word, then everything under it.
  function intro() {
    var h1 = document.querySelector("h1[data-split]");
    var words = [];
    if (h1) {
      var text = h1.textContent.replace(/\s+/g, " ").trim();
      h1.textContent = "";
      text.split(" ").forEach(function (word, i, all) {
        var span = document.createElement("span");
        span.className = "w";
        span.textContent = word;
        h1.appendChild(span);
        words.push(span);
        if (i < all.length - 1) h1.appendChild(document.createTextNode(" "));
      });
      h1.style.opacity = 1;
    }

    var tl = gsap.timeline({ defaults: { ease: "power3.out" } });
    tl.fromTo(".hero-icon", { opacity: 0, scale: 0.7, y: 24 }, { opacity: 1, scale: 1, y: 0, duration: 0.9, ease: "back.out(1.7)" })
      .fromTo(words, { opacity: 0, y: 38 }, { opacity: 1, y: 0, duration: 0.8, stagger: 0.06 }, "-=0.6")
      .fromTo(".hero .lead", { opacity: 0, y: 22 }, { opacity: 1, y: 0, duration: 0.7 }, "-=0.45")
      .fromTo(".hero .cta", { opacity: 0, y: 22 }, { opacity: 1, y: 0, duration: 0.7 }, "-=0.5")
      .fromTo(".hero .meta", { opacity: 0 }, { opacity: 1, duration: 0.6 }, "-=0.4")
      .fromTo(".hero-shot", { opacity: 0 }, { opacity: 1, duration: 0.9 }, "-=0.5");
  }

  // Everything below the fold rises in as it comes into view.
  function scrollReveals() {
    ScrollTrigger.batch(".card, .steps li, .chips li, details, .split-text, .split .shot, .download-icon", {
      start: "top 90%",
      once: true,
      onEnter: function (batch) {
        gsap.fromTo(batch, { opacity: 0, y: 44 }, { opacity: 1, y: 0, duration: 0.85, stagger: 0.09, ease: "power3.out", overwrite: true });
        batch.forEach(function (el) {
          var num = el.querySelector && el.querySelector(".num");
          if (num) gsap.from(num, { scale: 0.55, duration: 0.8, ease: "back.out(2.2)", transformOrigin: "0 50%" });
        });
      }
    });
  }

  // The hero window settles as you scroll; screenshots inside cards drift.
  function screenshots() {
    gsap.fromTo(".hero-shot",
      { y: 56, scale: 0.94, rotationX: 9, transformPerspective: 1200, transformOrigin: "50% 0%" },
      { y: 0, scale: 1, rotationX: 0, ease: "none",
        scrollTrigger: { trigger: ".hero-shot", start: "top 96%", end: "top 45%", scrub: 0.6 } });

    gsap.utils.toArray(".media .shot img").forEach(function (img) {
      gsap.fromTo(img, { y: 12 }, { y: -12, ease: "none",
        scrollTrigger: { trigger: img, start: "top bottom", end: "bottom top", scrub: true } });
    });
  }

  // The bar hides while reading down and returns on the way up; a thin line
  // shows how far through the page you are.
  function navigation() {
    var bar = document.querySelector(".nav-wrap");
    var hidden = false;
    function toggle(hide) {
      if (hide === hidden) return;
      hidden = hide;
      gsap.to(bar, { yPercent: hide ? -170 : 0, duration: 0.45, ease: "power3.out", overwrite: "auto" });
    }
    ScrollTrigger.create({
      start: 0,
      end: "max",
      onUpdate: function (self) {
        if (bar.contains(document.activeElement)) return toggle(false);
        toggle(self.direction === 1 && self.scroll() > 260);
      }
    });
    bar.addEventListener("focusin", function () { toggle(false); });

    gsap.to(".progress", { scaleX: 1, ease: "none",
      scrollTrigger: { start: 0, end: "max", scrub: 0.2 } });
  }

  // Answers ease open instead of appearing.
  function faq() {
    document.querySelectorAll("details").forEach(function (d) {
      d.addEventListener("toggle", function () {
        if (!d.open) return;
        var p = d.querySelector("p");
        if (p) gsap.fromTo(p, { opacity: 0, y: -10 }, { opacity: 1, y: 0, duration: 0.4, ease: "power2.out" });
      });
    });
  }

  // Buttons lean a little toward the pointer and spring back.
  function magnetic(selector) {
    gsap.utils.toArray(selector).forEach(function (el) {
      var x = gsap.quickTo(el, "x", { duration: 0.6, ease: "elastic.out(1, 0.45)" });
      var y = gsap.quickTo(el, "y", { duration: 0.6, ease: "elastic.out(1, 0.45)" });
      el.addEventListener("pointermove", function (e) {
        var r = el.getBoundingClientRect();
        x((e.clientX - (r.left + r.width / 2)) * 0.2);
        y((e.clientY - (r.top + r.height / 2)) * 0.28);
      });
      el.addEventListener("pointerleave", function () { x(0); y(0); });
    });
  }

  // The screenshot shifts slightly against its frame as the pointer moves.
  function depth(selector) {
    var frame = document.querySelector(selector);
    var img = frame && frame.querySelector("img");
    if (!img) return;
    gsap.set(img, { scale: 1.05 });
    var x = gsap.quickTo(img, "x", { duration: 0.9, ease: "power3.out" });
    var y = gsap.quickTo(img, "y", { duration: 0.9, ease: "power3.out" });
    frame.addEventListener("pointermove", function (e) {
      var r = frame.getBoundingClientRect();
      x(((e.clientX - r.left) / r.width - 0.5) * -22);
      y(((e.clientY - r.top) / r.height - 0.5) * -14);
    });
    frame.addEventListener("pointerleave", function () { x(0); y(0); });
  }
})();
