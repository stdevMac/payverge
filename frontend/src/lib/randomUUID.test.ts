import { randomUUID } from "./randomUUID";

const UUID_V4 = /^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/;

describe("randomUUID", () => {
  afterEach(() => {
    jest.restoreAllMocks();
  });

  it("uses crypto.randomUUID when the context provides it", () => {
    const spy = jest
      .spyOn(globalThis.crypto, "randomUUID")
      .mockReturnValue("11111111-1111-4111-8111-111111111111");
    expect(randomUUID()).toBe("11111111-1111-4111-8111-111111111111");
    expect(spy).toHaveBeenCalledTimes(1);
  });

  describe("without crypto.randomUUID (plain-http LAN install)", () => {
    const original = globalThis.crypto.randomUUID;

    beforeEach(() => {
      Object.defineProperty(globalThis.crypto, "randomUUID", {
        configurable: true,
        value: undefined,
      });
    });

    afterEach(() => {
      Object.defineProperty(globalThis.crypto, "randomUUID", {
        configurable: true,
        value: original,
      });
    });

    it("builds a v4 UUID from crypto.getRandomValues, never Math.random", () => {
      const mathRandom = jest.spyOn(Math, "random");
      const getRandomValues = jest.spyOn(globalThis.crypto, "getRandomValues");

      const id = randomUUID();

      expect(id).toMatch(UUID_V4);
      expect(getRandomValues).toHaveBeenCalledTimes(1);
      expect(mathRandom).not.toHaveBeenCalled();
    });

    it("sets the version and variant bits on fixed input", () => {
      jest
        .spyOn(globalThis.crypto, "getRandomValues")
        .mockImplementation(<T extends ArrayBufferView | null>(array: T): T => {
          (array as unknown as Uint8Array).fill(0xff);
          return array;
        });
      expect(randomUUID()).toBe("ffffffff-ffff-4fff-bfff-ffffffffffff");
    });
  });
});
