/* cosmos-interactions.js
   Proposta 2: interatividade — scroll spy, contadores, tabs de fase, canvas cosmos */
(function () {
  'use strict';

  // ===== SCROLL SPY NO TOC =====
  const tocLinks = document.querySelectorAll('.toc a[href^="#"]');
  const sections = Array.from(tocLinks).map(link => {
    const id = link.getAttribute('href').slice(1);
    return document.getElementById(id);
  }).filter(Boolean);

  function updateActiveToc() {
    const scrollPos = window.scrollY + 120;
    let activeIndex = sections.findIndex((sec, i) => {
      const next = sections[i + 1];
      const secTop = sec.offsetTop;
      const nextTop = next ? next.offsetTop : Infinity;
      return scrollPos >= secTop && scrollPos < nextTop;
    });
    if (activeIndex === -1) activeIndex = 0;
    
    tocLinks.forEach((link, i) => {
      link.classList.toggle('is-active', i === activeIndex);
    });
  }

  if (tocLinks.length && sections.length) {
    window.addEventListener('scroll', updateActiveToc, { passive: true });
    updateActiveToc();
  }

  // ===== CONTADORES ANIMADOS =====
  function animateCounter(el, target, duration = 1500, prefix = '', suffix = '') {
    const start = performance.now();
    const from = 0;
    
    function update(now) {
      const elapsed = now - start;
      const progress = Math.min(elapsed / duration, 1);
      const ease = 1 - Math.pow(1 - progress, 3);
      const current = from + (target - from) * ease;
      
      el.textContent = prefix + current.toFixed(2) + suffix;
      
      if (progress < 1) {
        requestAnimationFrame(update);
      } else {
        el.textContent = prefix + target.toFixed(2) + suffix;
      }
    }
    
    requestAnimationFrame(update);
  }

  const figureObserver = new IntersectionObserver((entries) => {
    entries.forEach(entry => {
      if (entry.isIntersecting && !entry.target.dataset.counted) {
        entry.target.dataset.counted = 'true';
        const valEl = entry.target.querySelector('.figure__val');
        if (valEl) {
          const text = valEl.textContent;
          const match = text.match(/[\d,.]+/);
          if (match) {
            const target = parseFloat(match[0].replace(',', '.'));
            const prefix = text.charAt(0) === '$' ? '$' : '';
            animateCounter(valEl, target, 1500, prefix);
          }
        }
      }
    });
  }, { threshold: 0.5, rootMargin: '0px 0px -10% 0px' });

  document.querySelectorAll('.figure').forEach(fig => figureObserver.observe(fig));

  // ===== TABS DE FASE PARA TABELA =====
  const tables = document.querySelectorAll('.table-scroll table');
  tables.forEach(table => {
    const caption = table.querySelector('caption');
    if (!caption || !caption.textContent.toLowerCase().includes('fase')) return;

    const container = table.closest('.table-scroll');
    const wrapper = document.createElement('div');
    wrapper.className = 'phase-table-wrapper';
    container.parentNode.insertBefore(wrapper, container);
    wrapper.appendChild(container);

    const tabContainer = document.createElement('div');
    tabContainer.className = 'phase-tabs';
    tabContainer.innerHTML = `
      <button class="phase-tab is-active" data-phase="1">Fase 1</button>
      <button class="phase-tab" data-phase="transition">Transição</button>
      <button class="phase-tab" data-phase="2">Fase 2</button>
    `;
    wrapper.insertBefore(tabContainer, container);

    const tabs = tabContainer.querySelectorAll('.phase-tab');
    tabs.forEach(tab => {
      tab.addEventListener('click', () => {
        tabs.forEach(t => t.classList.remove('is-active'));
        tab.classList.add('is-active');
        const phase = tab.dataset.phase;
        highlightTableColumns(table, phase);
      });
    });
  });

  function highlightTableColumns(table, phase) {
    const headerCells = table.querySelectorAll('thead th');
    const dataCells = table.querySelectorAll('tbody td');
    
    const colMap = { '1': 1, 'transition': 2, '2': 3 };
    const targetCol = colMap[phase];
    
    headerCells.forEach((th, i) => {
      th.style.opacity = i === targetCol ? '1' : '0.4';
      th.style.transition = 'opacity 0.3s ease';
    });
    
    dataCells.forEach((td, i) => {
      const colIndex = i % headerCells.length;
      td.style.opacity = colIndex === targetCol ? '1' : '0.4';
      td.style.transition = 'opacity 0.3s ease, background 0.3s ease';
      if (colIndex === targetCol) {
        td.style.background = 'oklch(0.22 0.015 62 / 0.3)';
      } else {
        td.style.background = 'transparent';
      }
    });
  }

  // ===== CANVAS COSMOS (NÓS INTERCONECTADOS) =====
  function initCosmosCanvas() {
    const hero = document.querySelector('.doc-head') || document.querySelector('.hero') || document.querySelector('header');
    if (!hero) return;

    const canvas = document.createElement('canvas');
    canvas.className = 'cosmos-canvas';
    hero.insertBefore(canvas, hero.firstChild);

    const ctx = canvas.getContext('2d');
    let width, height, dpr;
    let nodes = [];
    const NODE_COUNT = 25;
    const CONNECTION_DISTANCE = 120;
    const ANIMATION_SPEED = 0.3;

    function resize() {
      dpr = window.devicePixelRatio || 1;
      width = hero.offsetWidth;
      height = hero.offsetHeight;
      canvas.width = width * dpr;
      canvas.height = height * dpr;
      canvas.style.width = width + 'px';
      canvas.style.height = height + 'px';
      ctx.scale(dpr, dpr);
      initNodes();
    }

    function initNodes() {
      nodes = Array.from({ length: NODE_COUNT }, () => ({
        x: Math.random() * width,
        y: Math.random() * height,
        vx: (Math.random() - 0.5) * ANIMATION_SPEED,
        vy: (Math.random() - 0.5) * ANIMATION_SPEED,
        radius: Math.random() * 2 + 1,
        opacity: Math.random() * 0.5 + 0.2
      }));
    }

    function drawNode(node) {
      ctx.beginPath();
      ctx.arc(node.x, node.y, node.radius, 0, Math.PI * 2);
      ctx.fillStyle = `oklch(0.72 0.095 62 / ${node.opacity})`;
      ctx.fill();
    }

    function drawConnection(n1, n2, dist) {
      const opacity = (1 - dist / CONNECTION_DISTANCE) * 0.3;
      ctx.beginPath();
      ctx.moveTo(n1.x, n1.y);
      ctx.lineTo(n2.x, n2.y);
      ctx.strokeStyle = `oklch(0.72 0.095 62 / ${opacity})`;
      ctx.lineWidth = 0.5;
      ctx.stroke();
    }

    function animate() {
      ctx.clearRect(0, 0, width, height);

      nodes.forEach((node, i) => {
        node.x += node.vx;
        node.y += node.vy;

        if (node.x < 0 || node.x > width) node.vx *= -1;
        if (node.y < 0 || node.y > height) node.vy *= -1;

        drawNode(node);

        for (let j = i + 1; j < nodes.length; j++) {
          const other = nodes[j];
          const dist = Math.hypot(node.x - other.x, node.y - other.y);
          if (dist < CONNECTION_DISTANCE) {
            drawConnection(node, other, dist);
          }
        }
      });

      requestAnimationFrame(animate);
    }

    resize();
    animate();

    let resizeTimeout;
    window.addEventListener('resize', () => {
      clearTimeout(resizeTimeout);
      resizeTimeout = setTimeout(resize, 250);
    });
  }

  if (!window.matchMedia('(prefers-reduced-motion: reduce)').matches) {
    initCosmosCanvas();
  }

  // ===== CRONOGRAMA COM ESTADOS =====
  const roadmapSteps = document.querySelectorAll('#roadmap .step');
  roadmapSteps.forEach((step, index) => {
    step.classList.add('roadmap-item');
    
    // Simula: testnet é "active", decisões é "next"
    if (index === 0) step.classList.add('is-active');
    else if (index === 1) step.classList.add('is-active');
    else step.classList.add('is-pending');
  });

  // ===== ÍCONES NOS STEPS VIA DATA ATTRIBUTES =====
  const steps = document.querySelectorAll('.steps .step');
  const stepIcons = ['pay', 'prove', 'mine'];
  steps.forEach((step, index) => {
    if (stepIcons[index]) {
      step.setAttribute('data-icon', stepIcons[index]);
    }
    
    // Adicionar número grande como primeiro elemento
    const stepN = step.querySelector('.step__n');
    if (stepN && !step.querySelector('.step__number')) {
      const num = document.createElement('span');
      num.className = 'step__number';
      num.textContent = `0${index + 1}`;
      step.insertBefore(num, step.firstChild);
    }
  });

  // ===== REVEAL COM STAGGER =====
  document.querySelectorAll('.steps .step, .figures .figure, .notes .note').forEach((el, index) => {
    el.classList.add('reveal', `reveal-stagger-${(index % 3) + 1}`);
  });

  // ===== HOVER PREVIEW EM FORMULA =====
  const formula = document.querySelector('.formula');
  if (formula) {
    const tokens = formula.querySelectorAll('.tok');
    tokens.forEach((tok, i) => {
      tok.style.transitionDelay = `${i * 0.1}s`;
    });
  }

  // ===== MELHORIA NO SCROLL REVEAL EXISTENTE =====
  const existingReveals = document.querySelectorAll('.reveal');
  const revealObserver = new IntersectionObserver((entries) => {
    entries.forEach(entry => {
      if (entry.isIntersecting) {
        entry.target.classList.add('is-in');
        revealObserver.unobserve(entry.target);
      }
    });
  }, { threshold: 0.15, rootMargin: '0px 0px -5% 0px' });

  existingReveals.forEach(el => revealObserver.observe(el));

  // ===== NAVBAR TRANSPARENTE/BLUR =====
  const header = document.querySelector('.site-header');
  if (header) {
    let lastScroll = 0;
    window.addEventListener('scroll', () => {
      const current = window.scrollY;
      if (current > 100) {
        header.style.background = 'color-mix(in oklab, var(--paper) 95%, transparent)';
        header.style.boxShadow = '0 1px 0 var(--line), 0 4px 20px oklch(0 0 0 / 0.15)';
      } else {
        header.style.background = 'color-mix(in oklab, var(--paper) 88%, transparent)';
        header.style.boxShadow = 'none';
      }
      lastScroll = current;
    }, { passive: true });
  }

  console.log('[Cosmos] Interações carregadas');
})();
