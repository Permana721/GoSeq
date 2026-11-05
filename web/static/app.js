(function () {
  'use strict';

  function initApp() {
    const cards = document.querySelectorAll('.card-box, .home-card');
    cards.forEach(function (card, index) {
      card.style.opacity = '0';
      card.style.transform = 'translateY(14px)';
      card.style.transition = 'opacity 0.4s cubic-bezier(0.16, 1, 0.3, 1), transform 0.4s cubic-bezier(0.16, 1, 0.3, 1), border-color 0.2s ease, box-shadow 0.2s ease, background-color 0.2s ease';

      setTimeout(function () {
        card.style.opacity = '1';
        card.style.transform = 'translateY(0)';
      }, 40 + index * 100);
    });

    cards.forEach(function (card) {
      if (!card.hasAttribute('tabindex')) {
        card.setAttribute('tabindex', '0');
      }
      card.addEventListener('keydown', function (e) {
        if (e.key === 'Enter' || e.key === ' ') {
          e.preventDefault();
          const href = card.getAttribute('href');
          if (href) {
            window.location.href = href;
          }
        }
      });
    });

    const currentPath = window.location.pathname;
    const navLinks = document.querySelectorAll('.nav-links .nav-btn');
    navLinks.forEach(function (link) {
      const linkPath = link.getAttribute('href');
      if (linkPath === currentPath || (linkPath !== '/' && currentPath.startsWith(linkPath))) {
        link.classList.add('active');
      } else if (linkPath === '/' && currentPath === '/') {
        link.classList.add('active');
      }
    });

    document.querySelectorAll('a[href^="#"]').forEach(function (anchor) {
      anchor.addEventListener('click', function (e) {
        const href = this.getAttribute('href');
        if (href && href !== '#') {
          const target = document.querySelector(href);
          if (target) {
            e.preventDefault();
            target.scrollIntoView({ behavior: 'smooth' });
          }
        }
      });
    });

    initHeatmap();

    document.addEventListener('click', function (e) {
      const btn = e.target.closest('.copy-code-btn');
      if (!btn) return;
      const wrapper = btn.closest('.code-block-wrapper');
      if (!wrapper) return;
      const code = wrapper.querySelector('code');
      if (!code) return;
      navigator.clipboard.writeText(code.innerText).then(function () {
        const origText = btn.innerText;
        btn.innerText = 'Copied!';
        btn.classList.add('copied');
        setTimeout(function () {
          btn.innerText = origText;
          btn.classList.remove('copied');
        }, 2000);
      }).catch(function (err) {
        console.error('Failed to copy code: ', err);
      });
    });
  }

  function escapeHTML(str) {
    if (!str) return '';
    return str.replace(/[&<>"']/g, function (m) {
      switch (m) {
        case '&': return '&amp;';
        case '<': return '&lt;';
        case '>': return '&gt;';
        case '"': return '&quot;';
        case "'": return '&#39;';
        default: return m;
      }
    });
  }

  function initHeatmap() {
    const heatmapCard = document.querySelector('.heatmap-card');
    if (!heatmapCard) return;

    let tooltip = document.getElementById('heatmapTooltip');
    if (!tooltip) {
      tooltip = document.createElement('div');
      tooltip.id = 'heatmapTooltip';
      tooltip.className = 'heatmap-tooltip';
      document.body.appendChild(tooltip);
    }

    const cells = heatmapCard.querySelectorAll('.heatmap-cell:not(.future)');
    const detailsContainer = document.getElementById('heatmapDetails');
    const detailsTitle = document.getElementById('heatmapDetailsTitle');
    const detailsList = document.getElementById('heatmapDetailsList');
    const detailsClose = document.getElementById('heatmapDetailsClose');

    cells.forEach(function (cell) {
      cell.addEventListener('mouseenter', function () {
        const count = parseInt(cell.getAttribute('data-count') || '0', 10);
        const display = cell.getAttribute('data-display') || '';
        let label = 'No entries on ' + display;
        if (count === 1) {
          label = '1 entry on ' + display;
        } else if (count > 1) {
          label = count + ' entries on ' + display;
        }

        tooltip.textContent = label;
        const rect = cell.getBoundingClientRect();
        tooltip.style.left = (rect.left + rect.width / 2) + 'px';
        tooltip.style.top = rect.top + 'px';
        tooltip.classList.add('visible');
      });

      cell.addEventListener('mouseleave', function () {
        tooltip.classList.remove('visible');
      });

      cell.addEventListener('click', function (e) {
        const count = parseInt(cell.getAttribute('data-count') || '0', 10);
        if (count === 0) return;

        let articles = [];
        try {
          articles = JSON.parse(cell.getAttribute('data-articles') || '[]');
        } catch (err) {
          articles = [];
        }

        if (articles.length > 1 && detailsContainer) {
          e.preventDefault();
          cells.forEach(function (c) { c.classList.remove('selected'); });
          cell.classList.add('selected');

          const display = cell.getAttribute('data-display') || '';
          if (detailsTitle) {
            detailsTitle.textContent = articles.length + ' entries on ' + display;
          }
          if (detailsList) {
            detailsList.innerHTML = '';
            articles.forEach(function (art) {
              const li = document.createElement('li');
              li.className = 'heatmap-detail-item';
              li.innerHTML = '<a href="' + art.link + '" class="heatmap-detail-link">' +
                '<span class="heatmap-detail-section">' + escapeHTML(art.section) + '</span>' +
                '<span class="heatmap-detail-title">' + escapeHTML(art.title || art.slug) + '</span>' +
                '</a>';
              detailsList.appendChild(li);
            });
          }
          detailsContainer.style.display = 'block';
          detailsContainer.scrollIntoView({ behavior: 'smooth', block: 'nearest' });
        }
      });
    });

    if (detailsClose && detailsContainer) {
      detailsClose.addEventListener('click', function () {
        detailsContainer.style.display = 'none';
        cells.forEach(function (c) { c.classList.remove('selected'); });
      });
    }

    const scrollArea = heatmapCard.querySelector('.heatmap-scroll-container');
    if (scrollArea) {
      scrollArea.scrollLeft = scrollArea.scrollWidth;
    }

    const urlParams = new URLSearchParams(window.location.search);
    const dateParam = urlParams.get('date');
    if (dateParam) {
      const targetCell = heatmapCard.querySelector('.heatmap-cell[data-date="' + CSS.escape(dateParam) + '"]');
      if (targetCell) {
        targetCell.click();
      }
    }
  }

  if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', initApp);
  } else {
    initApp();
  }
})();
