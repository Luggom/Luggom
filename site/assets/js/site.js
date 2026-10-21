// Luggom — site behaviour. No dependencies, no build.
(function () {
  "use strict";

  // Mobile navigation toggle
  var toggle = document.querySelector(".nav-toggle");
  var nav = document.getElementById("primary-nav");
  if (toggle && nav) {
    toggle.addEventListener("click", function () {
      var open = nav.classList.toggle("is-open");
      toggle.setAttribute("aria-expanded", String(open));
    });
    nav.addEventListener("click", function (e) {
      if (e.target.tagName === "A") {
        nav.classList.remove("is-open");
        toggle.setAttribute("aria-expanded", "false");
      }
    });
  }

  // Scroll reveal
  var reduce = window.matchMedia("(prefers-reduced-motion: reduce)").matches;
  var items = document.querySelectorAll(".reveal");
  if (reduce || !("IntersectionObserver" in window)) {
    items.forEach(function (el) { el.classList.add("is-in"); });
  } else {
    var io = new IntersectionObserver(function (entries) {
      entries.forEach(function (entry) {
        if (entry.isIntersecting) {
          entry.target.classList.add("is-in");
          io.unobserve(entry.target);
        }
      });
    }, { rootMargin: "0px 0px -8% 0px", threshold: 0.12 });
    items.forEach(function (el) { io.observe(el); });
  }

  // Footer year
  document.querySelectorAll("[data-year]").forEach(function (el) {
    el.textContent = String(new Date().getFullYear());
  });

  // Preserve scroll position across full-page navigations (e.g. language switch),
  // so switching PT/EN leaves the visitor where they were instead of jumping to top.
  var SCROLL_KEY = "luggom:scroll";
  document.querySelectorAll(".lang a").forEach(function (link) {
    link.addEventListener("click", function (e) {
      if (link.hasAttribute("aria-current")) { e.preventDefault(); return; }
      var max = document.documentElement.scrollHeight - window.innerHeight;
      var ratio = max > 0 ? window.scrollY / max : 0;
      try { sessionStorage.setItem(SCROLL_KEY, String(ratio)); } catch (err) {}
    });
  });

  try {
    var stored = parseFloat(sessionStorage.getItem(SCROLL_KEY));
    if (!isNaN(stored)) {
      sessionStorage.removeItem(SCROLL_KEY);
      var prevBehavior = document.documentElement.style.scrollBehavior;
      document.documentElement.style.scrollBehavior = "auto";
      var restore = function () {
        var max = document.documentElement.scrollHeight - window.innerHeight;
        window.scrollTo(0, Math.round(stored * max));
      };
      restore();
      window.addEventListener("load", restore, { once: true });
      window.addEventListener("resize", restore, { once: true });
      setTimeout(function () { document.documentElement.style.scrollBehavior = prevBehavior; }, 0);
    }
  } catch (err) {}
})();
