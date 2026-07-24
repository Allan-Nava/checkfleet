(function () {
  'use strict';
  var root = document.documentElement;

  /* ---- theme toggle ---- */
  var toggle = document.querySelector('.theme-toggle');
  if (toggle) {
    toggle.addEventListener('click', function () {
      var next = root.getAttribute('data-theme') === 'dark' ? 'light' : 'dark';
      root.setAttribute('data-theme', next);
      try { localStorage.setItem('cf-theme', next); } catch (e) {}
    });
  }

  /* ---- mobile sidebar ---- */
  var navBtn = document.querySelector('.nav-toggle');
  var sidebar = document.getElementById('sidebar');
  var scrim = document.getElementById('scrim');
  function closeNav() {
    if (!sidebar) return;
    sidebar.classList.remove('open');
    if (scrim) scrim.hidden = true;
    if (navBtn) navBtn.setAttribute('aria-expanded', 'false');
  }
  if (navBtn && sidebar) {
    navBtn.addEventListener('click', function () {
      var open = sidebar.classList.toggle('open');
      if (scrim) scrim.hidden = !open;
      navBtn.setAttribute('aria-expanded', String(open));
    });
  }
  if (scrim) scrim.addEventListener('click', closeNav);

  /* ---- on-this-page TOC (built from headings) ---- */
  var toc = document.getElementById('toc');
  var article = document.querySelector('.doc');
  if (toc && article) {
    var heads = article.querySelectorAll('h2, h3');
    if (heads.length > 1) {
      var ul = document.createElement('ul');
      var title = document.createElement('p');
      title.className = 'toc-title';
      title.textContent = 'On this page';
      toc.appendChild(title);
      heads.forEach(function (h) {
        if (!h.id) {
          h.id = h.textContent.toLowerCase().trim()
            .replace(/[^\w\s-]/g, '').replace(/\s+/g, '-');
        }
        var li = document.createElement('li');
        li.className = 'toc-' + h.tagName.toLowerCase();
        var a = document.createElement('a');
        a.href = '#' + h.id;
        a.textContent = h.textContent;
        li.appendChild(a);
        ul.appendChild(li);
      });
      toc.appendChild(ul);

      /* scroll-spy */
      var links = toc.querySelectorAll('a');
      var byId = {};
      links.forEach(function (a) { byId[a.getAttribute('href').slice(1)] = a; });
      var spy = new IntersectionObserver(function (entries) {
        entries.forEach(function (e) {
          if (e.isIntersecting) {
            links.forEach(function (a) { a.classList.remove('active'); });
            var a = byId[e.target.id];
            if (a) a.classList.add('active');
          }
        });
      }, { rootMargin: '0px 0px -75% 0px' });
      heads.forEach(function (h) { spy.observe(h); });
    } else {
      toc.remove();
    }
  }

  /* ---- lightweight client-side search ---- */
  var input = document.getElementById('cf-search');
  var results = document.getElementById('cf-search-results');
  if (input && results) {
    var index = null, loading = false;
    function load() {
      if (index || loading) return;
      loading = true;
      var base = (window.CF_BASEURL || '');
      fetch(base + '/search.json')
        .then(function (r) { return r.json(); })
        .then(function (data) { index = data; })
        .catch(function () { index = []; });
    }
    input.addEventListener('focus', load);

    function render(items, q) {
      results.innerHTML = '';
      if (!items.length) {
        results.innerHTML = '<li class="empty">No results for “' + q + '”</li>';
      } else {
        items.slice(0, 8).forEach(function (it) {
          var li = document.createElement('li');
          var a = document.createElement('a');
          a.href = it.url;
          a.innerHTML = '<strong>' + it.title + '</strong><span>' +
            (it.content || '').slice(0, 90) + '…</span>';
          li.appendChild(a);
          results.appendChild(li);
        });
      }
      results.hidden = false;
    }

    input.addEventListener('input', function () {
      var q = input.value.trim().toLowerCase();
      if (!q || !index) { results.hidden = true; return; }
      var hits = index.filter(function (it) {
        return (it.title + ' ' + it.content).toLowerCase().indexOf(q) !== -1;
      });
      render(hits, input.value.trim());
    });

    document.addEventListener('click', function (e) {
      if (!results.contains(e.target) && e.target !== input) results.hidden = true;
    });
    input.addEventListener('keydown', function (e) {
      if (e.key === 'Escape') { results.hidden = true; input.blur(); }
    });
  }
})();
