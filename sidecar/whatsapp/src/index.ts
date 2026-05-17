import WebSocket from 'ws';

const WS_URL = process.env.WS_URL || 'ws://127.0.0.1:13001';
const INSTANCE_ID = process.env.INSTANCE_ID || 'default';
const HEADLESS = process.env.HEADLESS === 'true';

let ws: WebSocket | null = null;
let reconnectTimer: ReturnType<typeof setTimeout> | null = null;
let baileysReconnectTimer: ReturnType<typeof setTimeout> | null = null;

function send(msg: object) {
  if (ws?.readyState === WebSocket.OPEN) {
    ws.send(JSON.stringify(msg));
  }
}

function connect() {
  ws = new WebSocket(WS_URL, { headers: { 'X-Instance-Id': INSTANCE_ID } });

  ws.on('open', () => {
    send({ type: 'ping', instance_id: INSTANCE_ID });
    if (!HEADLESS) {
      startBaileys();
    }
  });

  ws.on('message', (data) => {
    try {
      const msg = JSON.parse(data.toString());
      handleCommand(msg);
    } catch {}
  });

  ws.on('close', () => {
    reconnectTimer = setTimeout(connect, 3000);
  });

  ws.on('error', () => {
    // close handler will reconnect
  });
}

function handleCommand(msg: { type: string; to?: string; text?: string; phone?: string }) {
  switch (msg.type) {
    case 'send':
      if (!HEADLESS && sock) {
        const jid = msg.to!.includes('@') ? msg.to! : msg.to! + '@s.whatsapp.net';
        sock.sendMessage(jid, { text: msg.text! }).catch((err: Error) => {
          console.error('[sidecar] send failed:', err.message);
        });
      }
      break;
    case 'request_qr':
      // Baileys will emit QR on next connection attempt
      break;
    case 'request_pairing_code':
      if (!HEADLESS && sock && msg.phone) {
        sock.requestPairingCode(msg.phone).then((code: string) => {
          send({ type: 'pairing_code', code });
        }).catch((err: Error) => {
          console.error('[sidecar] pairing code failed:', err.message);
          send({ type: 'error', reason: 'pairing_code_failed' });
        });
      }
      break;
    case 'pong':
      break;
  }
}

// Baileys socket — declared here so handleCommand can reference it
let sock: any = null;

async function startBaileys() {
  const DATA_DIR = process.env.DATA_DIR || `./data/${INSTANCE_ID}`;
  const { default: makeWASocket, DisconnectReason, useMultiFileAuthState } = await import('@whiskeysockets/baileys');
  const { default: pino } = await import('pino');
  const { Boom } = await import('@hapi/boom');

  const { state, saveCreds } = await useMultiFileAuthState(DATA_DIR);

  sock = makeWASocket({
    auth: state,
    logger: pino({ level: 'silent' }),
    browser: ['Qorven.ai', 'Chrome', '1.0.0'],
    printQRInTerminal: false,
    syncFullHistory: false,
  });

  sock.ev.on('creds.update', saveCreds);

  sock.ev.on('connection.update', async (update: any) => {
    const { connection, lastDisconnect, qr } = update;

    if (qr) {
      // Convert QR string to base64 PNG
      const QRCode = await import('qrcode');
      const png = await QRCode.toDataURL(qr);
      send({ type: 'qr', qr: png });
    }

    if (connection === 'open') {
      const phone = sock.user?.id?.split(':')[0] ?? '';
      const jid = sock.user?.id ?? '';
      send({ type: 'connected', phone, jid });
    }

    if (connection === 'close') {
      const statusCode = (lastDisconnect?.error as any)?.output?.statusCode;
      const shouldReconnect = statusCode !== DisconnectReason.loggedOut;
      send({ type: 'disconnected', reason: shouldReconnect ? 'connection_closed' : 'logged_out' });
      if (shouldReconnect) {
        baileysReconnectTimer = setTimeout(startBaileys, 3000);
      }
    }
  });

  sock.ev.on('messages.upsert', ({ messages, type }: any) => {
    if (type !== 'notify') return;
    for (const msg of messages) {
      if (msg.key.fromMe) continue;
      const from = msg.key.remoteJid ?? '';
      const body = msg.message?.conversation
        || msg.message?.extendedTextMessage?.text
        || '[media]';
      const fromName = msg.pushName ?? '';
      const chat = msg.key.remoteJid ?? from;
      send({
        type: 'message',
        id: msg.key.id,
        from,
        from_name: fromName,
        chat,
        body,
        ts: msg.messageTimestamp,
      });
    }
  });
}

function shutdown() {
  if (reconnectTimer) clearTimeout(reconnectTimer);
  if (baileysReconnectTimer) clearTimeout(baileysReconnectTimer);
  if (ws) ws.close();
  process.exit(0);
}
process.on('SIGTERM', shutdown);
process.on('SIGINT', shutdown);

connect();
