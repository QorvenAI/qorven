// Copyright 2026 Qorven AI. Licensed under Elastic License 2.0 (ELv2).

// Proxies audio blobs to the Go backend transcription endpoint.

const BACKEND = process.env.NEXT_PUBLIC_API_URL
  ? `${process.env.NEXT_PUBLIC_API_URL}/v1`
  : 'http://localhost:8080/v1';

function getToken(req: Request): string {
  const auth = req.headers.get('authorization');
  if (auth?.startsWith('Bearer ')) return auth.slice(7);
  return '';
}

export async function POST(req: Request) {
  const formData = await req.formData();
  const token = getToken(req);

  const upstream = await fetch(`${BACKEND}/audio/transcribe`, {
    method: 'POST',
    headers: { 'Authorization': `Bearer ${token}` },
    body: formData,
  });

  const result = await upstream.json().catch(() => ({ text: '' }));
  return Response.json(result, { status: upstream.ok ? 200 : upstream.status });
}
