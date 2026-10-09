class MockCanvasGradient {
  constructor(type, args) {
    this.type = type;
    this.args = args;
    this.stops = [];
  }

  addColorStop(offset, color) {
    this.stops.push([Number(offset), String(color)]);
  }
}

class MockCanvasPattern {
  setTransform(transform) {
    this.transform = transform;
  }
}

class ImageData {
  constructor(dataOrWidth, widthOrHeight, height) {
    if (dataOrWidth instanceof Uint8ClampedArray) {
      this.data = dataOrWidth;
      this.width = Number(widthOrHeight);
      this.height = Number(height ?? dataOrWidth.length / 4 / this.width);
      return;
    }
    this.width = Number(dataOrWidth);
    this.height = Number(widthOrHeight);
    this.data = new Uint8ClampedArray(this.width * this.height * 4);
  }
}

const DEFAULT_STATE = {
  fillStyle: "rgb(0, 0, 0)",
  strokeStyle: "rgb(0, 0, 0)",
  globalAlpha: 1,
  globalCompositeOperation: "source-over",
  lineWidth: 1,
  lineCap: "butt",
  lineJoin: "miter",
  miterLimit: 10,
  lineDashOffset: 0,
  shadowBlur: 0,
  shadowColor: "rgba(0, 0, 0, 0)",
  shadowOffsetX: 0,
  shadowOffsetY: 0,
  font: "10px sans-serif",
  textAlign: "start",
  textBaseline: "alphabetic",
  direction: "inherit",
  imageSmoothingEnabled: true,
  imageSmoothingQuality: "low",
  filter: "none",
};

class MockCanvasRenderingContext2D {
  constructor(canvas) {
    this.canvas = canvas;
    this.__calls = [];
    this.__stateStack = [];
    this.__transform = { a: 1, b: 0, c: 0, d: 1, e: 0, f: 0 };
    this.__lineDash = [];
    Object.assign(this, DEFAULT_STATE);
  }

  __record(method, args) {
    this.__calls.push({ method, args: Array.from(args) });
  }

  save() {
    this.__record("save", arguments);
    this.__stateStack.push({
      state: Object.fromEntries(
        Object.keys(DEFAULT_STATE).map((key) => [key, this[key]]),
      ),
      transform: { ...this.__transform },
      lineDash: [...this.__lineDash],
    });
  }

  restore() {
    this.__record("restore", arguments);
    const saved = this.__stateStack.pop();
    if (!saved) return;
    Object.assign(this, saved.state);
    this.__transform = saved.transform;
    this.__lineDash = saved.lineDash;
  }

  measureText(value) {
    this.__record("measureText", arguments);
    const pixels = Number.parseFloat(
      String(this.font).match(/([\d.]+)px/)?.[1] ?? "10",
    );
    const width = Array.from(String(value)).length * pixels * 0.52;
    return {
      width,
      actualBoundingBoxLeft: 0,
      actualBoundingBoxRight: width,
      actualBoundingBoxAscent: pixels * 0.8,
      actualBoundingBoxDescent: pixels * 0.2,
      fontBoundingBoxAscent: pixels * 0.8,
      fontBoundingBoxDescent: pixels * 0.2,
      emHeightAscent: pixels * 0.8,
      emHeightDescent: pixels * 0.2,
      hangingBaseline: pixels * 0.8,
      alphabeticBaseline: 0,
      ideographicBaseline: pixels * 0.2,
    };
  }

  createLinearGradient() {
    this.__record("createLinearGradient", arguments);
    return new MockCanvasGradient("linear", Array.from(arguments));
  }

  createRadialGradient() {
    this.__record("createRadialGradient", arguments);
    return new MockCanvasGradient("radial", Array.from(arguments));
  }

  createConicGradient() {
    this.__record("createConicGradient", arguments);
    return new MockCanvasGradient("conic", Array.from(arguments));
  }

  createPattern() {
    this.__record("createPattern", arguments);
    return new MockCanvasPattern();
  }

  getTransform() {
    this.__record("getTransform", arguments);
    return { ...this.__transform, is2D: true };
  }

  setTransform(a, b, c, d, e, f) {
    this.__record("setTransform", arguments);
    if (typeof a === "object") {
      this.__transform = {
        a: a.a ?? 1,
        b: a.b ?? 0,
        c: a.c ?? 0,
        d: a.d ?? 1,
        e: a.e ?? 0,
        f: a.f ?? 0,
      };
      return;
    }
    this.__transform = { a, b, c, d, e, f };
  }

  resetTransform() {
    this.__record("resetTransform", arguments);
    this.__transform = { a: 1, b: 0, c: 0, d: 1, e: 0, f: 0 };
  }

  setLineDash(segments) {
    this.__record("setLineDash", arguments);
    this.__lineDash = Array.from(segments);
  }

  getLineDash() {
    this.__record("getLineDash", arguments);
    return [...this.__lineDash];
  }

  createImageData(widthOrData, height) {
    this.__record("createImageData", arguments);
    if (widthOrData instanceof ImageData) {
      return new ImageData(widthOrData.width, widthOrData.height);
    }
    return new ImageData(widthOrData, height);
  }

  getImageData(_x, _y, width, height) {
    this.__record("getImageData", arguments);
    return new ImageData(width, height);
  }

  isPointInPath() {
    this.__record("isPointInPath", arguments);
    return false;
  }

  isPointInStroke() {
    this.__record("isPointInStroke", arguments);
    return false;
  }
}

for (const method of [
  "arc",
  "arcTo",
  "beginPath",
  "bezierCurveTo",
  "clearRect",
  "clip",
  "closePath",
  "drawFocusIfNeeded",
  "drawImage",
  "ellipse",
  "fill",
  "fillRect",
  "fillText",
  "lineTo",
  "moveTo",
  "putImageData",
  "quadraticCurveTo",
  "rect",
  "reset",
  "rotate",
  "roundRect",
  "scale",
  "scrollPathIntoView",
  "stroke",
  "strokeRect",
  "strokeText",
  "transform",
  "translate",
]) {
  MockCanvasRenderingContext2D.prototype[method] = function () {
    this.__record(method, arguments);
  };
}

class Canvas {
  constructor(width = 300, height = 150) {
    this.width = width;
    this.height = height;
    this.__context2d = null;
  }

  getContext(kind) {
    if (kind !== "2d") return null;
    if (!this.__context2d) {
      this.__context2d = new MockCanvasRenderingContext2D(this);
    }
    return this.__context2d;
  }

  toDataURL(type = "image/png") {
    return `data:${type};base64,`;
  }

  toBuffer(callback) {
    const buffer = Buffer.from("");
    if (typeof callback === "function") {
      callback(null, buffer);
      return undefined;
    }
    return buffer;
  }
}

class Image {
  constructor() {
    this.width = 0;
    this.height = 0;
    this.naturalWidth = 0;
    this.naturalHeight = 0;
    this.onload = null;
    this.onerror = null;
    this._src = null;
  }

  set src(value) {
    this._src = value;
  }

  get src() {
    return this._src;
  }
}

function createCanvas(width, height) {
  return new Canvas(width, height);
}

async function loadImage(source) {
  const image = new Image();
  image.src = source;
  return image;
}

module.exports = {
  Canvas,
  CanvasRenderingContext2D: MockCanvasRenderingContext2D,
  Image,
  ImageData,
  createCanvas,
  loadImage,
};
