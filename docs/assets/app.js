(function () {
  "use strict";

  var API = "https://api.github.com/repos/ilyasaftr/OpenPrintCast/releases/latest";
  var CACHE_KEY = "opc-latest-release";
  var CACHE_MS = 10 * 60 * 1000;

  // Fade sections in as they scroll into view; without support, show them.
  var reveal = document.querySelectorAll(".reveal");
  // What is on screen at load shows at once, so the first view never waits.
  reveal.forEach(function (el) {
    if (el.getBoundingClientRect().top < window.innerHeight) el.classList.add("in");
  });
  if ("IntersectionObserver" in window) {
    var io = new IntersectionObserver(function (entries) {
      entries.forEach(function (entry) {
        if (entry.isIntersecting || entry.target.classList.contains("in")) {
          entry.target.classList.add("in");
          io.unobserve(entry.target);
        }
      });
    }, { rootMargin: "0px 0px -8% 0px", threshold: 0.08 });
    reveal.forEach(function (el) { io.observe(el); });
  } else {
    reveal.forEach(function (el) { el.classList.add("in"); });
  }

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
})();
