// Lobby balls — slow falling red & yellow orbs for the start page
// Now with a "hole" behind the start card so it never obstructs the form.

(() => {
    const formEl = document.querySelector('.start-form');
    if (!formEl) return; // only on start screen
    if (window.matchMedia('(prefers-reduced-motion: reduce)').matches) return;

    // ---- config ----
    const CFG = {
        density: 0.00016,
        minR: 8, maxR: 18,
        minVy: 12, maxVy: 28,
        swayAmp: 18,
        swaySpeed: [0.6, 1.2],
        alpha: [0.35, 0.75],
        shadow: 14,
        colors: ['#f59e0b', '#ef4444'],
        zIndex: 2,
        holePadding: 14,   // extra empty margin inside the card
    };

    // The "hole" is the first .card that contains the form.
    const cardEl = formEl.closest('.card');

    // ---- canvas layer ----
    const dpr = Math.max(1, Math.min(2, window.devicePixelRatio || 1));
    const c = document.createElement('canvas');
    const ctx = c.getContext('2d');

    Object.assign(c.style, {
        position: 'fixed',
        inset: '0',
        pointerEvents: 'none',
        zIndex: String(CFG.zIndex),
    });
    document.body.appendChild(c);

    function resizeCanvas() {
        c.width  = Math.floor(innerWidth * dpr);
        c.height = Math.floor(innerHeight * dpr);
        c.style.width = innerWidth + 'px';
        c.style.height = innerHeight + 'px';
        ctx.setTransform(dpr, 0, 0, dpr, 0, 0);
        targetCount = Math.round(innerWidth * innerHeight * CFG.density);
        updateHoleRect();
    }

    // compute the hole rect (in CSS px)
    const hole = { x: 0, y: 0, w: 0, h: 0, r: 16 }; // r: approx border-radius
    function updateHoleRect() {
        if (!cardEl) { hole.w = hole.h = 0; return; }
        const b = cardEl.getBoundingClientRect();
        const p = CFG.holePadding;
        hole.x = Math.max(0, b.left)  + p;
        hole.y = Math.max(0, b.top)   + p;
        hole.w = Math.max(0, b.width  - p*2);
        hole.h = Math.max(0, b.height - p*2);
        // try to read actual border radius if present
        const cs = getComputedStyle(cardEl);
        const br = parseFloat(cs.borderRadius || cs.borderTopLeftRadius || '16') || 16;
        hole.r = br;
    }

    let targetCount = 0;
    resizeCanvas();
    addEventListener('resize', resizeCanvas);

    // track card size changes too
    if (window.ResizeObserver && cardEl) {
        const ro = new ResizeObserver(() => updateHoleRect());
        ro.observe(cardEl);
    }

    // ---- particles ----
    const balls = [];
    const rand = (a, b) => a + Math.random() * (b - a);
    let t0 = performance.now() / 1000;

    function makeBall(spawnTop = false) {
        const r = rand(CFG.minR, CFG.maxR);
        const x = rand(-CFG.swayAmp, innerWidth + CFG.swayAmp);
        const y = spawnTop ? rand(-60, -10) : rand(0, innerHeight);
        return {
            x0: x, y, r,
            vy: rand(CFG.minVy, CFG.maxVy),
            phase: rand(0, Math.PI * 2),
            swayHz: rand(CFG.swaySpeed[0], CFG.swaySpeed[1]),
            color: CFG.colors[Math.random() < 0.5 ? 0 : 1],
            alpha: rand(CFG.alpha[0], CFG.alpha[1]),
        };
    }

    for (let i = 0; i < targetCount; i++) balls.push(makeBall(false));

    // parallax
    let parallaxX = 0, parallaxY = 0;
    addEventListener('pointermove', e => {
        const nx = (e.clientX / innerWidth - 0.5) * 8;
        const ny = (e.clientY / innerHeight - 0.5) * 8;
        parallaxX += (nx - parallaxX) * 0.08;
        parallaxY += (ny - parallaxY) * 0.08;
    });

    // helpers
    function hexToRgb(hex) {
        const h = hex.replace('#', '');
        const n = parseInt(h.length === 3 ? h.split('').map(c => c + c).join('') : h, 16);
        return { r: (n >> 16) & 255, g: (n >> 8) & 255, b: n & 255 };
    }

    // quick circle-vs-rounded-rect overlap check
    function circleOverlapsRoundedRect(cx, cy, cr, rx, ry, rw, rh, rr) {
        // clamp circle center to rect bounds (with rounded corners)
        const nx = Math.max(rx + rr, Math.min(cx, rx + rw - rr));
        const ny = Math.max(ry + rr, Math.min(cy, ry + rh - rr));
        // if we're in the straight center area, it's a hit
        if (cx >= rx + rr && cx <= rx + rw - rr && cy >= ry && cy <= ry + rh) return true;
        if (cy >= ry + rr && cy <= ry + rh - rr && cx >= rx && cx <= rx + rw) return true;
        // otherwise distance to the nearest corner arc
        const dx = cx - nx;
        const dy = cy - ny;
        return (dx*dx + dy*dy) <= (cr + rr) * (cr + rr);
    }

    // public API
    window.LobbyBalls = {
        stop() { running = false; },
        start() { if (!running){ running = true; requestAnimationFrame(loop); } },
        destroy() { running = false; removeEventListener('resize', resizeCanvas); c.remove(); }
    };

    // loop
    let running = true;
    document.addEventListener('visibilitychange', () => {
        running = document.visibilityState === 'visible';
        if (running) requestAnimationFrame(loop);
    });

    function loop(nowMs) {
        if (!running) return;
        const t = nowMs / 1000;
        const dt = Math.min(0.032, t - t0);
        t0 = t;

        while (balls.length < targetCount) balls.push(makeBall(true));
        if (balls.length > targetCount) balls.length = targetCount;

        ctx.clearRect(0, 0, innerWidth, innerHeight);
        ctx.save();
        ctx.shadowBlur = CFG.shadow;
        ctx.globalCompositeOperation = 'lighter';

        for (let i = 0; i < balls.length; i++) {
            const b = balls[i];
            const sway = Math.sin((t + b.phase) * b.swayHz * Math.PI * 2) * CFG.swayAmp;
            const cx = b.x0 + sway + parallaxX * 0.6;
            b.y += b.vy * dt + parallaxY * 0.06;
            const cy = b.y;

            // wrap to top when off-screen
            if (cy - b.r > innerHeight + 12) {
                const keepX = Math.random() < 0.7;
                b.y = rand(-80, -20);
                if (!keepX) b.x0 = rand(-CFG.swayAmp, innerWidth + CFG.swayAmp);
                b.vy = rand(CFG.minVy, CFG.maxVy);
                b.phase = rand(0, Math.PI * 2);
                b.swayHz = rand(CFG.swaySpeed[0], CFG.swaySpeed[1]);
                b.alpha = rand(CFG.alpha[0], CFG.alpha[1]);
                b.color = CFG.colors[Math.random() < 0.5 ? 0 : 1];
            }

            // 🚫 skip drawing if circle overlaps the card "hole"
            if (hole.w > 0 && hole.h > 0 &&
                circleOverlapsRoundedRect(cx, cy, b.r, hole.x, hole.y, hole.w, hole.h, hole.r)) {
                continue;
            }

            // draw glossy orb
            const { r, g, b: bb } = hexToRgb(b.color);
            const grad = ctx.createRadialGradient(cx - b.r*0.4, cy - b.r*0.5, b.r*0.1, cx, cy, b.r);
            grad.addColorStop(0, `rgba(255,255,255,${0.6 * b.alpha})`);
            grad.addColorStop(0.5, `rgba(${r},${g},${bb},${0.85 * b.alpha})`);
            grad.addColorStop(1, `rgba(${r},${g},${bb},${0.0})`);

            ctx.fillStyle = grad;
            ctx.shadowColor = `rgba(${r},${g},${bb},${0.5 * b.alpha})`;
            ctx.beginPath();
            ctx.arc(cx, cy, b.r, 0, Math.PI * 2);
            ctx.fill();
        }

        ctx.restore();
        requestAnimationFrame(loop);
    }

    requestAnimationFrame(loop);
})();
