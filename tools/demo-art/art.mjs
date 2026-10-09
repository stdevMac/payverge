// Project-made illustrations for the demo restaurants' menus and storefronts.
//
// Every picture is drawn here from SVG primitives, so the demo data ships no
// photograph, stock image or generated image of unknown origin. The output is
// Apache-2.0 like the rest of the repository. render.mjs turns each entry of
// ART into the JPEG the demo seed embeds (backend/internal/demo/assets/).

// ---------------------------------------------------------------------------
// Deterministic helpers

function rng(seed) {
  let s = seed >>> 0;
  return () => {
    s = (s + 0x6d2b79f5) >>> 0;
    let t = s;
    t = Math.imul(t ^ (t >>> 15), t | 1);
    t ^= t + Math.imul(t ^ (t >>> 7), t | 61);
    return ((t ^ (t >>> 14)) >>> 0) / 4294967296;
  };
}

const f = (n) => Number(n.toFixed(1));

// A smooth closed blob around (cx, cy): rx/ry radii, `wobble` 0..1.
function blob(cx, cy, rx, ry, { seed = 1, wobble = 0.12, points = 10, rot = 0 } = {}) {
  const r = rng(seed);
  const pts = [];
  for (let i = 0; i < points; i++) {
    const a = (i / points) * Math.PI * 2;
    const k = 1 + (r() * 2 - 1) * wobble;
    const x = Math.cos(a) * rx * k;
    const y = Math.sin(a) * ry * k;
    const c = Math.cos(rot), s = Math.sin(rot);
    pts.push([cx + x * c - y * s, cy + x * s + y * c]);
  }
  // Catmull-Rom to cubic Bezier.
  let d = `M${f(pts[0][0])} ${f(pts[0][1])}`;
  for (let i = 0; i < points; i++) {
    const p0 = pts[(i - 1 + points) % points], p1 = pts[i], p2 = pts[(i + 1) % points], p3 = pts[(i + 2) % points];
    const c1 = [p1[0] + (p2[0] - p0[0]) / 6, p1[1] + (p2[1] - p0[1]) / 6];
    const c2 = [p2[0] - (p3[0] - p1[0]) / 6, p2[1] - (p3[1] - p1[1]) / 6];
    d += ` C${f(c1[0])} ${f(c1[1])} ${f(c2[0])} ${f(c2[1])} ${f(p2[0])} ${f(p2[1])}`;
  }
  return d + 'Z';
}

// Scattered small dots (herbs, salt, chili) inside an ellipse.
function specks(cx, cy, rx, ry, n, colors, { seed = 7, size = [2, 5] } = {}) {
  const r = rng(seed);
  let out = '';
  for (let i = 0; i < n; i++) {
    const a = r() * Math.PI * 2, d = Math.sqrt(r());
    const x = cx + Math.cos(a) * rx * d, y = cy + Math.sin(a) * ry * d;
    const w = size[0] + r() * (size[1] - size[0]);
    const c = colors[Math.floor(r() * colors.length)];
    out += `<rect x="${f(x)}" y="${f(y)}" width="${f(w)}" height="${f(w * (0.5 + r() * 0.6))}" rx="${f(w / 3)}" fill="${c}" transform="rotate(${f(r() * 180)} ${f(x)} ${f(y)})"/>`;
  }
  return out;
}

// Diagonal grill marks clipped to a shape id.
function grill(clipId, x0, y0, x1, y1, { gap = 42, angle = -32, width = 9, color = '#2b160c', opacity = 0.72 } = {}) {
  let lines = '';
  for (let x = x0 - 400; x < x1 + 400; x += gap) {
    lines += `<rect x="${x}" y="${y0 - 400}" width="${width}" height="${y1 - y0 + 800}" rx="${width / 2}" fill="${color}" opacity="${opacity}"/>`;
  }
  const cx = (x0 + x1) / 2, cy = (y0 + y1) / 2;
  return `<g clip-path="url(#${clipId})"><g transform="rotate(${angle} ${cx} ${cy})" filter="url(#soften)">${lines}</g></g>`;
}

// ---------------------------------------------------------------------------
// Shared scene pieces

const DEFS = `
<defs>
  <filter id="grain" x="0" y="0" width="100%" height="100%">
    <feTurbulence type="fractalNoise" baseFrequency="0.012 0.32" numOctaves="3" seed="4" result="n"/>
    <feColorMatrix in="n" type="matrix" values="0 0 0 0 0.24  0 0 0 0 0.13  0 0 0 0 0.07  0 0 0 0.55 0"/>
    <feComposite in2="SourceGraphic" operator="in"/>
  </filter>
  <filter id="crumb" x="-5%" y="-5%" width="110%" height="110%">
    <feTurbulence type="fractalNoise" baseFrequency="0.09" numOctaves="2" seed="9" result="n"/>
    <feDiffuseLighting in="n" surfaceScale="2.4" lighting-color="#ffe7c2" result="l"><feDistantLight azimuth="235" elevation="48"/></feDiffuseLighting>
    <feComposite in="l" in2="SourceGraphic" operator="arithmetic" k1="1.15" k2="0" k3="0" k4="0" result="lit"/>
    <feComposite in="lit" in2="SourceGraphic" operator="in"/>
  </filter>
  <filter id="sear" x="-5%" y="-5%" width="110%" height="110%">
    <feTurbulence type="fractalNoise" baseFrequency="0.035" numOctaves="3" seed="3" result="n"/>
    <feDiffuseLighting in="n" surfaceScale="3" lighting-color="#ffd9b8" result="l"><feDistantLight azimuth="225" elevation="55"/></feDiffuseLighting>
    <feComposite in="l" in2="SourceGraphic" operator="arithmetic" k1="1.05" k2="0" k3="0" k4="0" result="lit"/>
    <feComposite in="lit" in2="SourceGraphic" operator="in"/>
  </filter>
  <filter id="soften"><feGaussianBlur stdDeviation="2.2"/></filter>
  <filter id="blur6"><feGaussianBlur stdDeviation="6"/></filter>
  <filter id="shadow" x="-20%" y="-20%" width="140%" height="150%">
    <feGaussianBlur in="SourceAlpha" stdDeviation="16"/>
    <feOffset dx="10" dy="18" result="b"/>
    <feComponentTransfer><feFuncA type="linear" slope="0.45"/></feComponentTransfer>
    <feMerge><feMergeNode/><feMergeNode in="SourceGraphic"/></feMerge>
  </filter>
  <filter id="shadowSm" x="-20%" y="-20%" width="140%" height="150%">
    <feGaussianBlur in="SourceAlpha" stdDeviation="6"/>
    <feOffset dx="4" dy="7" result="b"/>
    <feComponentTransfer><feFuncA type="linear" slope="0.4"/></feComponentTransfer>
    <feMerge><feMergeNode/><feMergeNode in="SourceGraphic"/></feMerge>
  </filter>
  <radialGradient id="vignette" cx="50%" cy="45%" r="75%">
    <stop offset="55%" stop-color="#000" stop-opacity="0"/>
    <stop offset="100%" stop-color="#000" stop-opacity="0.45"/>
  </radialGradient>
  <radialGradient id="plateWell" cx="45%" cy="40%" r="60%">
    <stop offset="0%" stop-color="#fffdf8"/>
    <stop offset="100%" stop-color="#ece5d8"/>
  </radialGradient>
  <linearGradient id="plateRim" x1="0" y1="0" x2="1" y2="1">
    <stop offset="0%" stop-color="#ffffff"/>
    <stop offset="100%" stop-color="#e2dacb"/>
  </linearGradient>
  <linearGradient id="board" x1="0" y1="0" x2="1" y2="1">
    <stop offset="0%" stop-color="#c9935e"/>
    <stop offset="100%" stop-color="#a8723f"/>
  </linearGradient>
  <radialGradient id="beef" cx="40%" cy="35%" r="70%">
    <stop offset="0%" stop-color="#9a5530"/>
    <stop offset="70%" stop-color="#6e3519"/>
    <stop offset="100%" stop-color="#4a210e"/>
  </radialGradient>
  <radialGradient id="golden" cx="40%" cy="35%" r="70%">
    <stop offset="0%" stop-color="#f2c06a"/>
    <stop offset="75%" stop-color="#d9913a"/>
    <stop offset="100%" stop-color="#b8702a"/>
  </radialGradient>
  <radialGradient id="cheese" cx="45%" cy="40%" r="65%">
    <stop offset="0%" stop-color="#fff1c2"/>
    <stop offset="80%" stop-color="#f4d27a"/>
    <stop offset="100%" stop-color="#d9a24a"/>
  </radialGradient>
  <radialGradient id="iron" cx="45%" cy="40%" r="60%">
    <stop offset="0%" stop-color="#3a3836"/>
    <stop offset="100%" stop-color="#1b1a19"/>
  </radialGradient>
  <linearGradient id="wine" x1="0" y1="0" x2="0" y2="1">
    <stop offset="0%" stop-color="#7a1030"/>
    <stop offset="100%" stop-color="#3d0617"/>
  </linearGradient>
  <linearGradient id="glass" x1="0" y1="0" x2="1" y2="0">
    <stop offset="0%" stop-color="#ffffff" stop-opacity="0.10"/>
    <stop offset="35%" stop-color="#ffffff" stop-opacity="0.38"/>
    <stop offset="100%" stop-color="#ffffff" stop-opacity="0.06"/>
  </linearGradient>
</defs>`;

function table(w, h, { tone = 'walnut' } = {}) {
  const base = tone === 'walnut' ? '#6b4128' : '#7c4d2e';
  let planks = '';
  const ph = 148;
  for (let y = -40, i = 0; y < h; y += ph, i++) {
    const shade = i % 2 ? 0.06 : 0;
    planks += `<rect x="0" y="${y}" width="${w}" height="${ph}" fill="#000" opacity="${shade}"/><rect x="0" y="${y + ph - 2}" width="${w}" height="3" fill="#3b2215" opacity="0.55"/>`;
  }
  return `<rect width="${w}" height="${h}" fill="${base}"/>${planks}<rect width="${w}" height="${h}" fill="${base}" filter="url(#grain)" opacity="0.9"/>`;
}

function vignette(w, h) {
  return `<rect width="${w}" height="${h}" fill="url(#vignette)"/>`;
}

function plate(cx, cy, r) {
  return `<g filter="url(#shadow)"><circle cx="${cx}" cy="${cy}" r="${r}" fill="url(#plateRim)"/></g>
  <circle cx="${cx}" cy="${cy}" r="${r * 0.74}" fill="url(#plateWell)"/>
  <circle cx="${cx}" cy="${cy}" r="${r * 0.74}" fill="none" stroke="#d9d0c0" stroke-width="2"/>
  <circle cx="${cx}" cy="${cy}" r="${r - 6}" fill="none" stroke="#ffffff" stroke-width="3" opacity="0.8"/>`;
}

function boardRect(x, y, w, h, rx = 26) {
  return `<g filter="url(#shadow)"><rect x="${x}" y="${y}" width="${w}" height="${h}" rx="${rx}" fill="url(#board)"/></g>
  <rect x="${x}" y="${y}" width="${w}" height="${h}" rx="${rx}" fill="#a06a38" filter="url(#grain)" opacity="0.55"/>
  <rect x="${x + 10}" y="${y + 10}" width="${w - 20}" height="${h - 20}" rx="${rx - 8}" fill="none" stroke="#e0b07a" stroke-width="2" opacity="0.5"/>`;
}

function napkin(x, y, w, h, rot = -8) {
  let stripes = '';
  for (let i = 0; i < 3; i++) stripes += `<rect x="${x + 22 + i * 12}" y="${y}" width="5" height="${h}" fill="#3f6f8f" opacity="0.65"/>`;
  return `<g transform="rotate(${rot} ${x + w / 2} ${y + h / 2})"><g filter="url(#shadowSm)"><rect x="${x}" y="${y}" width="${w}" height="${h}" rx="10" fill="#f4efe6"/></g>${stripes}
  <rect x="${x}" y="${y}" width="${w}" height="${h}" rx="10" fill="none" stroke="#e1d8c8" stroke-width="2"/></g>`;
}

function cutlery(x, y, len = 300, rot = -8) {
  return `<g transform="rotate(${rot} ${x} ${y})" filter="url(#shadowSm)">
    <rect x="${x}" y="${y}" width="16" height="${len}" rx="8" fill="#d8dadc"/>
    <rect x="${x + 3}" y="${y}" width="5" height="${len}" rx="2.5" fill="#f5f6f7" opacity="0.8"/>
    <g transform="translate(${x + 48} ${y})">
      <rect x="0" y="${len * 0.32}" width="14" height="${len * 0.68}" rx="7" fill="#d8dadc"/>
      <path d="M-8 0 h30 v${len * 0.22} q0 ${len * 0.1} -15 ${len * 0.12} q-15 -0.02 -15 -${len * 0.12} z" fill="#d8dadc"/>
      <rect x="-4" y="0" width="4" height="${len * 0.17}" fill="#9fa3a6"/><rect x="5" y="0" width="4" height="${len * 0.17}" fill="#9fa3a6"/><rect x="14" y="0" width="4" height="${len * 0.17}" fill="#9fa3a6"/>
    </g>
  </g>`;
}

function chimichurri(cx, cy, r = 54, seed = 5) {
  return `<g filter="url(#shadowSm)"><circle cx="${cx}" cy="${cy}" r="${r}" fill="#efe8dc"/></g>
  <circle cx="${cx}" cy="${cy}" r="${r * 0.78}" fill="#4f6b22"/>
  <circle cx="${cx}" cy="${cy}" r="${r * 0.78}" fill="#7c9a3a" opacity="0.5" filter="url(#crumb)"/>
  ${specks(cx, cy, r * 0.7, r * 0.7, 26, ['#a2c25a', '#2f4a14', '#c4462b', '#e8d27a'], { seed, size: [3, 7] })}
  <ellipse cx="${cx - r * 0.25}" cy="${cy - r * 0.3}" rx="${r * 0.3}" ry="${r * 0.12}" fill="#fff" opacity="0.25"/>`;
}

function lemon(cx, cy, r = 46, rot = 0) {
  return `<g transform="rotate(${rot} ${cx} ${cy})" filter="url(#shadowSm)">
    <path d="M${cx - r} ${cy} A${r} ${r} 0 0 0 ${cx + r} ${cy} Z" fill="#f2d33b"/>
    <path d="M${cx - r * 0.84} ${cy + 2} A${r * 0.84} ${r * 0.84} 0 0 0 ${cx + r * 0.84} ${cy + 2} Z" fill="#fbeea0"/>
    ${[0.2, 0.4, 0.6, 0.8].map((t) => { const a = Math.PI * t; return `<line x1="${cx}" y1="${cy + 4}" x2="${f(cx + Math.cos(a) * r * 0.78)}" y2="${f(cy + Math.sin(a) * r * 0.78)}" stroke="#f2d33b" stroke-width="3"/>`; }).join('')}
  </g>`;
}

function fries(x, y, n = 9, seed = 11) {
  const r = rng(seed);
  let out = '';
  for (let i = 0; i < n; i++) {
    const fx = x + r() * 150, fy = y + r() * 130, len = 90 + r() * 70, rot = -60 + r() * 120;
    out += `<g transform="rotate(${f(rot)} ${f(fx)} ${f(fy)})"><rect x="${f(fx)}" y="${f(fy)}" width="${f(len)}" height="18" rx="5" fill="#e8b04c"/><rect x="${f(fx)}" y="${f(fy)}" width="${f(len)}" height="7" rx="3" fill="#f6d488" opacity="0.8"/></g>`;
  }
  return `<g filter="url(#shadowSm)">${out}</g>`;
}

function scene(w, h, body, { tone } = {}) {
  return `<svg xmlns="http://www.w3.org/2000/svg" width="${w}" height="${h}" viewBox="0 0 ${w} ${h}">${DEFS}${table(w, h, { tone })}${body}${vignette(w, h)}</svg>`;
}

// ---------------------------------------------------------------------------
// Dishes (1024 x 768)

const W = 1024, H = 768;

function steak(cx, cy, rx, ry, { seed, rot = 0, clip, fat = true, bone = false }) {
  const d = blob(cx, cy, rx, ry, { seed, wobble: 0.1, rot });
  const fatD = blob(cx, cy, rx * 1.03, ry * 1.06, { seed, wobble: 0.1, rot });
  return `<clipPath id="${clip}"><path d="${d}"/></clipPath>
  <g filter="url(#shadowSm)">${fat ? `<path d="${fatD}" fill="#f1dcc0"/>` : ''}<path d="${d}" fill="url(#beef)"/></g>
  <path d="${d}" fill="#7a3a1a" filter="url(#sear)" opacity="0.85"/>
  ${grill(clip, cx - rx, cy - ry, cx + rx, cy + ry)}
  ${bone ? `<ellipse cx="${cx + rx * 0.55}" cy="${cy - ry * 0.35}" rx="${rx * 0.16}" ry="${ry * 0.2}" fill="#f6ead7" stroke="#d8c3a2" stroke-width="4"/>` : ''}
  ${specks(cx, cy, rx * 0.7, ry * 0.7, 30, ['#ffffff', '#f3efe8'], { seed: seed + 3, size: [2.5, 5] })}`;
}

const dishes = {
  'carta/provoleta.jpg': () => scene(W, H, `
    ${napkin(700, 60, 300, 380, 12)}${cutlery(790, 70, 330, 12)}
    ${plate(450, 400, 330)}
    <g filter="url(#shadow)"><rect x="575" y="185" width="230" height="46" rx="23" fill="#232120" transform="rotate(-28 575 208)"/></g>
    <circle cx="430" cy="410" r="235" fill="url(#iron)"/>
    <circle cx="430" cy="410" r="235" fill="none" stroke="#4a4744" stroke-width="6"/>
    <path d="${blob(430, 410, 190, 180, { seed: 21, wobble: 0.05 })}" fill="#c9852f" opacity="0.6"/>
    <path d="${blob(430, 405, 175, 168, { seed: 22, wobble: 0.04 })}" fill="url(#cheese)"/>
    ${[[360, 340, 34], [500, 330, 28], [470, 470, 40], [350, 470, 26], [540, 420, 22], [420, 400, 18]].map(([x, y, r], i) => `<path d="${blob(x, y, r, r * 0.8, { seed: 30 + i, wobble: 0.3 })}" fill="#b5652a" opacity="0.75" filter="url(#soften)"/>`).join('')}
    ${specks(430, 405, 150, 140, 70, ['#5d7a2a', '#3e5a1a', '#b23a1e'], { seed: 23, size: [3, 6] })}
  `),

  'carta/empanadas.jpg': () => scene(W, H, `
    ${napkin(40, 420, 260, 300, -14)}
    ${plate(540, 384, 330)}
    ${[[430, 300, -20, 1], [640, 290, 15, 2], [540, 470, 0, 3]].map(([x, y, rot, s]) => {
      let crimp = '';
      for (let i = 0; i <= 12; i++) {
        const a = Math.PI + (i / 12) * Math.PI;
        crimp += `<ellipse cx="${f(x + Math.cos(a) * 128)}" cy="${f(y + Math.sin(a) * 92 + 6)}" rx="15" ry="11" fill="#c47a2c" transform="rotate(${f((a * 180) / Math.PI + 90)} ${f(x + Math.cos(a) * 128)} ${f(y + Math.sin(a) * 92 + 6)})"/>`;
      }
      return `<g transform="rotate(${rot} ${x} ${y})" filter="url(#shadowSm)">
        <path d="M${x - 135} ${y + 20} A135 105 0 0 1 ${x + 135} ${y + 20} Q${x} ${y + 60} ${x - 135} ${y + 20} Z" fill="url(#golden)"/>
        <path d="M${x - 135} ${y + 20} A135 105 0 0 1 ${x + 135} ${y + 20} Q${x} ${y + 60} ${x - 135} ${y + 20} Z" fill="#d9913a" filter="url(#crumb)" opacity="0.55"/>
        ${crimp}
        <ellipse cx="${x - 30}" cy="${y - 40}" rx="40" ry="14" fill="#fff3d0" opacity="0.35"/>
        ${specks(x, y - 10, 60, 30, s + 3, ['#7a3e12'], { seed: 40 + s, size: [3, 5] })}
      </g>`;
    }).join('')}
    ${chimichurri(880, 130, 62, 8)}
  `),

  'carta/mollejas.jpg': () => scene(W, H, `
    ${cutlery(110, 120, 360, -6)}
    ${plate(560, 390, 320)}
    ${[[470, 330, 70, 52, 1], [600, 300, 64, 50, 2], [660, 420, 72, 54, 3], [520, 460, 66, 50, 4], [560, 380, 50, 40, 5]].map(([x, y, rx, ry, s]) =>
      `<g filter="url(#shadowSm)"><path d="${blob(x, y, rx, ry, { seed: 50 + s, wobble: 0.22, points: 9 })}" fill="url(#golden)"/></g>
       <path d="${blob(x, y, rx, ry, { seed: 50 + s, wobble: 0.22, points: 9 })}" fill="#c8802f" filter="url(#crumb)" opacity="0.7"/>`).join('')}
    ${lemon(760, 280, 52, 140)}${lemon(420, 520, 46, -20)}
    ${specks(560, 390, 180, 150, 40, ['#ffffff', '#e7e2d6'], { seed: 59, size: [3, 5] })}
    ${specks(560, 390, 180, 150, 24, ['#5d7a2a'], { seed: 58, size: [3, 6] })}
  `),

  'carta/choripan.jpg': () => scene(W, H, `
    ${boardRect(110, 160, 800, 440)}
    <g filter="url(#shadowSm)">
      <rect x="190" y="290" width="640" height="190" rx="95" fill="#d9a35b"/>
      <rect x="190" y="290" width="640" height="190" rx="95" fill="#c98c45" filter="url(#crumb)" opacity="0.6"/>
    </g>
    <rect x="215" y="350" width="590" height="78" rx="39" fill="#8a3a1c"/>
    <rect x="215" y="350" width="590" height="78" rx="39" fill="#7a3418" filter="url(#sear)" opacity="0.8"/>
    ${[300, 400, 500, 600, 700].map((x) => `<rect x="${x}" y="352" width="10" height="74" rx="5" fill="#2b140a" opacity="0.6" transform="rotate(18 ${x} 389)"/>`).join('')}
    ${specks(510, 388, 260, 26, 60, ['#4f6b22', '#7c9a3a', '#c4462b'], { seed: 61, size: [4, 9] })}
    ${chimichurri(820, 150, 60, 62)}
  `),

  'carta/bife-de-chorizo.jpg': () => scene(W, H, `
    ${boardRect(90, 110, 840, 540)}
    ${steak(470, 390, 250, 165, { seed: 71, rot: -0.12, clip: 'c-bife' })}
    ${chimichurri(820, 210, 64, 72)}
    <g filter="url(#shadowSm)"><circle cx="820" cy="540" r="44" fill="#efe8dc"/></g>
    ${specks(820, 540, 30, 30, 30, ['#ffffff', '#e8e2d8'], { seed: 73, size: [3, 6] })}
  `),

  'carta/ojo-de-bife.jpg': () => scene(W, H, `
    ${napkin(40, 60, 250, 330, -10)}${cutlery(100, 380, 320, -10)}
    ${plate(590, 390, 330)}
    ${steak(590, 390, 210, 190, { seed: 81, clip: 'c-ojo', bone: true })}
    <path d="M520 360 q40 -30 90 -6 q30 20 70 4" fill="none" stroke="#f3dcc0" stroke-width="5" opacity="0.55"/>
    <path d="M540 440 q50 20 110 -10" fill="none" stroke="#f3dcc0" stroke-width="4" opacity="0.45"/>
  `),

  'carta/asado-de-tira.jpg': () => scene(W, H, `
    ${boardRect(70, 120, 880, 520)}
    ${[0, 1].map((k) => {
      const y = 250 + k * 190, id = `c-tira-${k}`;
      const d = `M150 ${y} h720 q20 0 20 20 v110 q0 20 -20 20 h-720 q-20 0 -20 -20 v-110 q0 -20 20 -20z`;
      return `<clipPath id="${id}"><path d="${d}"/></clipPath>
        <g filter="url(#shadowSm)"><path d="${d}" fill="url(#beef)"/></g>
        <path d="${d}" fill="#7a3a1a" filter="url(#sear)" opacity="0.85"/>
        ${grill(id, 130, y, 890, y + 150, { gap: 60, angle: 60 })}
        ${[230, 400, 570, 740].map((x) => `<ellipse cx="${x + k * 30}" cy="${y + 75}" rx="30" ry="24" fill="#f4e6cf" stroke="#cfb48a" stroke-width="5"/><ellipse cx="${x + k * 30}" cy="${y + 75}" rx="12" ry="9" fill="#d9b98a"/>`).join('')}`;
    }).join('')}
    ${specks(510, 380, 360, 200, 50, ['#ffffff'], { seed: 91, size: [3, 5] })}
  `),

  'carta/entrana.jpg': () => scene(W, H, `
    ${plate(512, 390, 340)}
    <clipPath id="c-entrana"><path d="M230 330 C330 250 520 300 640 290 C760 280 820 320 820 380 C820 440 720 470 600 470 C450 470 330 520 250 470 C190 430 180 370 230 330Z"/></clipPath>
    <g filter="url(#shadowSm)"><path d="M230 330 C330 250 520 300 640 290 C760 280 820 320 820 380 C820 440 720 470 600 470 C450 470 330 520 250 470 C190 430 180 370 230 330Z" fill="url(#beef)"/></g>
    <path d="M230 330 C330 250 520 300 640 290 C760 280 820 320 820 380 C820 440 720 470 600 470 C450 470 330 520 250 470 C190 430 180 370 230 330Z" fill="#7a3a1a" filter="url(#sear)" opacity="0.85"/>
    ${grill('c-entrana', 180, 250, 830, 520, { gap: 46, angle: 28 })}
    ${specks(520, 380, 260, 70, 70, ['#4f6b22', '#7c9a3a', '#c4462b'], { seed: 101, size: [4, 8] })}
    ${specks(520, 380, 280, 80, 30, ['#ffffff'], { seed: 102, size: [3, 5] })}
  `),

  'carta/parrillada.jpg': () => scene(W, H, `
    <g filter="url(#shadow)"><rect x="90" y="90" width="844" height="590" rx="20" fill="#1f1d1c"/></g>
    ${Array.from({ length: 15 }, (_, i) => `<rect x="${120 + i * 54}" y="110" width="12" height="550" rx="6" fill="#4d4a47"/>`).join('')}
    <rect x="90" y="90" width="844" height="590" rx="20" fill="none" stroke="#5c5855" stroke-width="10"/>
    ${steak(330, 260, 160, 105, { seed: 111, clip: 'c-p1', rot: 0.1 })}
    ${[[620, 220], [620, 330]].map(([x, y], i) => `<g filter="url(#shadowSm)"><rect x="${x}" y="${y}" width="240" height="78" rx="39" fill="#8a3a1c"/></g><rect x="${x}" y="${y}" width="240" height="78" rx="39" fill="#7a3418" filter="url(#sear)" opacity="0.8"/>${[60, 120, 180].map((dx) => `<rect x="${x + dx}" y="${y + 4}" width="9" height="70" rx="4" fill="#2b140a" opacity="0.6"/>`).join('')}`).join('')}
    <g filter="url(#shadowSm)"><rect x="210" y="430" width="250" height="84" rx="42" fill="#3a1714"/></g>
    <rect x="210" y="430" width="250" height="84" rx="42" fill="#2f1210" filter="url(#sear)" opacity="0.7"/>
    ${[[600, 500, 1], [700, 540, 2], [770, 470, 3]].map(([x, y, s]) => `<g filter="url(#shadowSm)"><path d="${blob(x, y, 56, 42, { seed: 120 + s, wobble: 0.22, points: 9 })}" fill="url(#golden)"/></g>`).join('')}
    <circle cx="300" cy="590" r="58" fill="url(#cheese)"/>
    ${specks(300, 590, 46, 46, 18, ['#5d7a2a', '#b23a1e'], { seed: 125, size: [3, 6] })}
  `),

  'carta/matambrito.jpg': () => scene(W, H, `
    ${napkin(760, 420, 240, 300, 10)}
    ${boardRect(90, 130, 720, 500)}
    ${(() => {
      const d = blob(440, 380, 280, 170, { seed: 131, wobble: 0.08, points: 12 });
      return `<clipPath id="c-mat"><path d="${d}"/></clipPath><g filter="url(#shadowSm)"><path d="${d}" fill="#d08a4a"/></g>
      <path d="${d}" fill="#b56f34" filter="url(#sear)" opacity="0.8"/>${grill('c-mat', 160, 210, 720, 550, { gap: 52, angle: -20, opacity: 0.55 })}`;
    })()}
    ${lemon(660, 240, 56, 200)}${lemon(250, 540, 50, -30)}
    ${specks(440, 380, 220, 120, 40, ['#5d7a2a', '#ffffff'], { seed: 133, size: [3, 6] })}
  `),

  'carta/milanesa-napolitana.jpg': () => scene(W, H, milanesa(W, H)),

  'carta/sorrentinos.jpg': () => scene(W, H, `
    ${cutlery(80, 140, 360, -4)}
    ${plate(560, 390, 330)}
    <path d="${blob(560, 390, 230, 215, { seed: 141, wobble: 0.06 })}" fill="#e58a62"/>
    <path d="${blob(560, 390, 230, 215, { seed: 141, wobble: 0.06 })}" fill="#d9744d" filter="url(#crumb)" opacity="0.35"/>
    ${[[470, 310], [640, 300], [560, 400], [450, 470], [670, 460]].map(([x, y], i) => `
      <g filter="url(#shadowSm)"><circle cx="${x}" cy="${y}" r="78" fill="#f6e3b8"/></g>
      ${Array.from({ length: 22 }, (_, k) => { const a = (k / 22) * Math.PI * 2; return `<circle cx="${f(x + Math.cos(a) * 74)}" cy="${f(y + Math.sin(a) * 74)}" r="8" fill="#ead39c"/>`; }).join('')}
      <circle cx="${x}" cy="${y}" r="52" fill="#fbecc8"/>
      <ellipse cx="${x - 18}" cy="${y - 20}" rx="22" ry="9" fill="#fff" opacity="0.5"/>
      ${specks(x, y, 40, 40, 4, ['#e58a62'], { seed: 150 + i, size: [8, 16] })}`).join('')}
    ${specks(560, 390, 200, 190, 40, ['#4f7a2a', '#2f5a14'], { seed: 149, size: [4, 9] })}
  `),

  'carta/flan-casero.jpg': () => scene(W, H, `
    ${napkin(60, 80, 260, 320, -12)}
    ${plate(560, 400, 300)}
    <g filter="url(#shadowSm)"><circle cx="520" cy="410" r="150" fill="#e8b85a"/></g>
    <circle cx="520" cy="410" r="150" fill="#d9a24a" filter="url(#crumb)" opacity="0.3"/>
    <circle cx="520" cy="410" r="112" fill="#8a4a16"/>
    <circle cx="520" cy="410" r="112" fill="#a65d1f" filter="url(#sear)" opacity="0.6"/>
    <ellipse cx="490" cy="370" rx="50" ry="16" fill="#fff" opacity="0.35"/>
    <path d="${blob(560, 520, 130, 26, { seed: 161, wobble: 0.2 })}" fill="#7a3e12" opacity="0.8"/>
    <g filter="url(#shadowSm)"><path d="${blob(700, 300, 70, 56, { seed: 162, wobble: 0.15 })}" fill="#b5702e"/></g>
    <path d="${blob(700, 300, 50, 38, { seed: 163, wobble: 0.15 })}" fill="#c98140" opacity="0.8"/>
    <g filter="url(#shadowSm)"><path d="${blob(720, 470, 70, 60, { seed: 164, wobble: 0.25 })}" fill="#fffaf0"/></g>
    <path d="M690 460 q30 -30 60 0" fill="none" stroke="#efe6d4" stroke-width="5"/>
    ${cutlery(870, 200, 330, 8)}
  `),

  'carta/malbec.jpg': () => scene(W, H, `
    <g filter="url(#shadow)">
      <rect x="610" y="150" width="150" height="520" rx="40" fill="#1d2b22"/>
      <rect x="650" y="40" width="70" height="170" rx="18" fill="#1d2b22"/>
      <rect x="645" y="40" width="80" height="40" rx="10" fill="#5a1022"/>
    </g>
    <rect x="628" y="170" width="16" height="480" rx="8" fill="#ffffff" opacity="0.12"/>
    <rect x="620" y="330" width="130" height="190" rx="8" fill="#f2ebdc"/>
    <rect x="636" y="350" width="98" height="4" fill="#5a1022"/>
    <text x="685" y="410" text-anchor="middle" font-family="Georgia, serif" font-size="30" fill="#2a1d14">Malbec</text>
    <text x="685" y="446" text-anchor="middle" font-family="Georgia, serif" font-size="16" fill="#6b5a48">Mesa de muestra</text>
    <text x="685" y="490" text-anchor="middle" font-family="Georgia, serif" font-size="16" fill="#6b5a48">2022</text>
    <g filter="url(#shadow)">
      <path d="M260 140 h220 q10 150 -60 210 q-30 24 -50 24 q-20 0 -50 -24 q-70 -60 -60 -210z" fill="url(#glass)" stroke="#ffffff" stroke-opacity="0.55" stroke-width="3"/>
    </g>
    <path d="M272 238 h196 q-10 90 -48 112 q-30 24 -50 24 q-20 0 -50 -24 q-38 -22 -48 -112z" fill="url(#wine)"/>
    <ellipse cx="370" cy="238" rx="98" ry="12" fill="#9b1a3e"/>
    <rect x="362" y="372" width="16" height="220" fill="#ffffff" opacity="0.45"/>
    <ellipse cx="370" cy="600" rx="110" ry="22" fill="#ffffff" opacity="0.4"/>
    <path d="M300 160 q-6 70 10 140" fill="none" stroke="#ffffff" stroke-width="6" opacity="0.5"/>
    ${specks(150, 660, 80, 30, 12, ['#7a1030'], { seed: 171, size: [3, 6] })}
  `),
};

function milanesa(w, h, { cx = w * 0.46, cy = h * 0.54, scale = 1 } = {}) {
  const s = scale;
  const d = blob(cx - 40 * s, cy + 10 * s, 250 * s, 175 * s, { seed: 181, wobble: 0.1, points: 12, rot: -0.15 });
  return `
    ${napkin(20, h - 330, 260, 300, -16)}
    ${plate(cx + 20 * s, cy, 350 * s)}
    <g filter="url(#shadowSm)"><path d="${d}" fill="#b8712c"/></g>
    <path d="${d}" fill="#c47b30" filter="url(#crumb)"/>
    <path d="${blob(cx - 50 * s, cy + 5 * s, 175 * s, 120 * s, { seed: 182, wobble: 0.12, rot: -0.15 })}" fill="#e9a3a3"/>
    <path d="${blob(cx - 40 * s, cy + 10 * s, 160 * s, 110 * s, { seed: 183, wobble: 0.18, points: 12 })}" fill="#fbf0cf"/>
    <path d="${blob(cx - 40 * s, cy + 5 * s, 110 * s, 72 * s, { seed: 184, wobble: 0.2 })}" fill="#c8361f"/>
    ${specks(cx - 40 * s, cy + 5 * s, 90 * s, 60 * s, 24, ['#3e6a1a', '#e9573a'], { seed: 185, size: [3, 7] })}
    ${fries(cx + 150 * s, cy - 150 * s, 10, 186)}
    ${lemon(cx + 230 * s, cy + 150 * s, 50, -25)}
  `;
}

// ---------------------------------------------------------------------------
// Promo tile (1024 x 1024) and venue heroes (1600 x 900). The heroes carry no
// lettering: the storefront sets the venue name over them.

const promos = {
  'promos/offer-almuerzo-15.jpg': () => scene(1024, 1024, `
    ${milanesa(1024, 1024, { cx: 470, cy: 690, scale: 0.95 })}
    <text x="96" y="230" font-family="Georgia, serif" font-size="52" fill="#fbf4e6">Almuerzo de semana</text>
    <text x="96" y="290" font-family="Helvetica, Arial, sans-serif" font-size="26" fill="#f0dcc0" opacity="0.9">Lunes a viernes, de 12 a 16</text>
    <g filter="url(#shadow)"><circle cx="800" cy="230" r="150" fill="#1a4f57"/></g>
    <circle cx="800" cy="230" r="136" fill="none" stroke="#f3e7cf" stroke-width="6"/>
    <text x="800" y="230" text-anchor="middle" font-family="Helvetica, Arial, sans-serif" font-weight="800" font-size="92" fill="#f3e7cf">15%</text>
    <text x="800" y="310" text-anchor="middle" font-family="Helvetica, Arial, sans-serif" font-weight="800" font-size="70" fill="#f3e7cf">OFF</text>
  `),
};

function bunting(w, y, colors) {
  let out = '';
  const n = Math.floor(w / 80);
  for (let i = 0; i < n; i++) {
    const x = i * 80 + 20;
    out += `<path d="M${x} ${y + (i % 2) * 6} l60 0 l-30 52 z" fill="${colors[i % colors.length]}"/>`;
  }
  return `<path d="M0 ${y} Q${w / 2} ${y + 30} ${w} ${y}" stroke="#2b1d14" stroke-width="3" fill="none"/>${out}`;
}

function bodegonHero() {
  const w = 1600, h = 900;
  // Interior of a fictional bodegón: tiled wall, shelves with bottles, a long table.
  let tiles = '';
  for (let y = 0; y < 520; y += 60) for (let x = 0; x < w; x += 60) tiles += `<rect x="${x + 2}" y="${y + 2}" width="56" height="56" rx="4" fill="${(x / 60 + y / 60) % 2 ? '#e7dcc4' : '#efe6d2'}"/>`;
  let bottles = '';
  const r = rng(201);
  for (let i = 0; i < 22; i++) {
    const x = 120 + i * 62 + r() * 10, hh = 90 + r() * 40, col = ['#2d4a33', '#5a1022', '#6b4a1a', '#1d2b22'][i % 4];
    bottles += `<rect x="${f(x)}" y="${f(250 - hh)}" width="36" height="${f(hh)}" rx="10" fill="${col}"/><rect x="${f(x + 11)}" y="${f(250 - hh - 34)}" width="14" height="40" rx="5" fill="${col}"/><rect x="${f(x + 4)}" y="${f(250 - hh * 0.6)}" width="28" height="26" fill="#f2ebdc" opacity="0.9"/>`;
  }
  let cloth = '';
  for (let y = 600; y < h; y += 40) for (let x = 0; x < w; x += 40) if ((x / 40 + y / 40) % 2) cloth += `<rect x="${x}" y="${y}" width="40" height="40" fill="#b8352b" opacity="0.85"/>`;
  return `<svg xmlns="http://www.w3.org/2000/svg" width="${w}" height="${h}" viewBox="0 0 ${w} ${h}">${DEFS}
    <rect width="${w}" height="${h}" fill="#f4ecdc"/>${tiles}
    <rect x="0" y="250" width="${w}" height="22" fill="#6b4128"/>${bottles}
    <rect x="0" y="440" width="${w}" height="30" fill="#6b4128"/>
    ${bunting(w, 300, ['#1a6b6a', '#b77333', '#b8352b', '#e8c45a'])}
    <rect x="0" y="560" width="${w}" height="${h - 560}" fill="#f6f0e4"/>${cloth}
    <rect x="0" y="560" width="${w}" height="44" fill="#000" opacity="0.08"/>
    ${plate(420, 720, 150)}${plate(1180, 720, 150)}
    <g transform="translate(-50 -40)">${'' /* bread basket */}
      <g filter="url(#shadow)"><ellipse cx="850" cy="740" rx="150" ry="90" fill="#a0703c"/></g>
      ${[[800, 720], [880, 710], [840, 760], [910, 760]].map(([x, y], i) => `<path d="${blob(x, y, 52, 32, { seed: 210 + i, wobble: 0.1 })}" fill="url(#golden)"/>`).join('')}
    </g>
    ${vignette(w, h)}</svg>`;
}

function parrillaHero() {
  const w = 1600, h = 900;
  let bricks = '';
  for (let y = 0, row = 0; y < 560; y += 46, row++) for (let x = row % 2 ? -50 : 0; x < w; x += 100) bricks += `<rect x="${x + 3}" y="${y + 3}" width="94" height="40" rx="4" fill="${['#8a4a2e', '#7d4128', '#94533a'][(x / 100 + row) % 3 | 0]}"/>`;
  let flames = '';
  const r = rng(301);
  for (let i = 0; i < 26; i++) {
    const x = 220 + i * 46 + r() * 20, hh = 80 + r() * 120;
    flames += `<path d="M${f(x)} 760 q-26 -${f(hh * 0.5)} 6 -${f(hh)} q8 ${f(hh * 0.4)} 28 ${f(hh * 0.55)} q10 -${f(hh * 0.2)} 2 -${f(hh * 0.45)} q40 ${f(hh * 0.5)} 4 ${f(hh * 0.9)} z" fill="${i % 3 ? '#f08a24' : '#f7c548'}" opacity="0.85"/>`;
  }
  return `<svg xmlns="http://www.w3.org/2000/svg" width="${w}" height="${h}" viewBox="0 0 ${w} ${h}">${DEFS}
    <rect width="${w}" height="${h}" fill="#2a1a12"/>${bricks}
    <rect width="${w}" height="560" fill="#000" opacity="0.35"/>
    <rect x="0" y="560" width="${w}" height="${h - 560}" fill="#1b1411"/>
    <g filter="url(#blur6)">${flames}</g>
    <rect x="160" y="600" width="1280" height="18" rx="9" fill="#3d3a37"/>
    ${Array.from({ length: 25 }, (_, i) => `<rect x="${180 + i * 51}" y="580" width="10" height="60" rx="5" fill="#4d4a47"/>`).join('')}
    ${steak(420, 560, 150, 70, { seed: 311, clip: 'h-1' })}
    ${steak(800, 560, 170, 74, { seed: 312, clip: 'h-2', rot: 0.08 })}
    ${[[1110, 540], [1110, 590]].map(([x, y]) => `<rect x="${x}" y="${y}" width="230" height="44" rx="22" fill="#8a3a1c"/>`).join('')}
    ${vignette(w, h)}</svg>`;
}

const venues = {
  'venues/bodegon-mesa-larga-hero.jpg': bodegonHero,
  'venues/parrilla-quebracho-azul-hero.jpg': parrillaHero,
};

const SIZES = { carta: [1024, 768], promos: [1024, 1024], venues: [1600, 900] };

export const ART = Object.fromEntries(
  Object.entries({ ...dishes, ...promos, ...venues }).map(([path, draw]) => {
    const [w, h] = SIZES[path.split('/')[0]];
    return [path, { width: w, height: h, svg: draw }];
  }),
);
