import { ConfigService } from './config.service';

function mockFetch(status: number, body: unknown): void {
  globalThis.fetch = vi.fn().mockResolvedValue({
    ok: status >= 200 && status < 300,
    status,
    json: async () => body,
  } as Response);
}

describe('ConfigService', () => {
  const originalFetch = globalThis.fetch;
  afterEach(() => {
    globalThis.fetch = originalFetch;
  });

  it('ucitava gatewayUrl i uklanja / na kraju', async () => {
    mockFetch(200, { gatewayUrl: ' http://localhost:8080/ ' });
    const s = new ConfigService();
    await s.load();
    expect(s.gatewayUrl).toBe('http://localhost:8080');
  });

  it('javlja gresku kad config.json nema gatewayUrl', async () => {
    mockFetch(200, {});
    await expect(new ConfigService().load()).rejects.toThrow('gatewayUrl');
  });

  it('javlja gresku kad config.json ne postoji', async () => {
    mockFetch(404, null);
    await expect(new ConfigService().load()).rejects.toThrow('404');
  });

  it('ne daje adresu pre ucitavanja', () => {
    expect(() => new ConfigService().gatewayUrl).toThrow();
  });
});
