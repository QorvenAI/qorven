import { createServer } from 'http';
import { WebSocketServer } from 'ws';

test('sidecar connects to WS_URL and sends ping', (done) => {
  const httpServer = createServer();
  const wss = new WebSocketServer({ server: httpServer });
  httpServer.listen(0, '127.0.0.1', () => {
    const port = (httpServer.address() as any).port;
    process.env.WS_URL = `ws://127.0.0.1:${port}`;
    process.env.INSTANCE_ID = 'test-instance';
    process.env.DATA_DIR = '/tmp/test-wa-sidecar';
    process.env.HEADLESS = 'true';

    wss.on('connection', (ws) => {
      ws.on('message', (data) => {
        const msg = JSON.parse(data.toString());
        if (msg.type === 'ping') {
          httpServer.close();
          done();
        }
      });
    });

    // Reset module cache so env vars are re-read at module load time
    jest.resetModules();
    // Import after env is set
    require('./index');
  });
}, 10000);
