# External-Facing Apps

A Qorven app can serve a PUBLIC surface (for logged-out visitors on the internet) that writes to the app's own internal DB via a controlled bridge, while an internal AUTHED admin page manages the data. Example: a customer booking page → rows land in the app's table → an admin app page (authed) manages bookings.

**Default-deny:** nothing is public unless you (a) flag it `public` in the manifest AND (b) an admin publishes the app externally AND (c) a tunnel is running (see the host's Settings → Network → Internet exposure).

## 1. Mark pages/tools public in `app.yaml`
```yaml
frontend:
  bundle: ui/frontend/bundle.js
  pages:
    - id: book      # public booking page
      label: Book
      path: book
      public: true   # ← exposed on the public surface
    - id: admin     # internal admin view (NOT public → only in /apps/{slug})
      label: Manage
      path: admin
tools:
  - name: submit_booking
    description: Create a booking from the public page
    command: tools/submit_booking.sh
    public: true     # ← callable from the public bridge
    parameters:
      type: object
      properties:
        name:  { type: string }
        email: { type: string }
      required: [name, email]
  - name: list_bookings   # admin-only (no public flag) → NOT on the bridge
    description: List bookings
    command: tools/list_bookings.sh
```

## 2. Public page bundle — target `window.__QorvenPublic`
A public page is served by a STANDALONE host (no auth, no Next.js). It exposes a SLIM SDK — NOT the internal `__QorvenApp`/`__QorvenUI`:
```js
(function(){
  var Q = window.__QorvenPublic;       // { React, h, request, register }
  var h = Q.h, useState = Q.React.useState;
  function BookPage(){
    var s = useState(''); var name = s[0], setName = s[1];
    function submit(){
      // request() is hard-scoped to THIS app's bridge: /a/{slug}/tools/*
      Q.request('/a/my-app/tools/submit_booking', {method:'POST', body: JSON.stringify({args:{name:name, email:''}})})
        .then(function(r){ /* show confirmation */ });
    }
    return h('div', {style:{padding:'24px'}},
      h('input', {value:name, onInput:function(e){setName(e.target.value)}, placeholder:'Your name'}),
      h('button', {onClick:submit}, 'Book'));
  }
  Q.register({ pages: [{ id:'book', path:'book', component: BookPage }] });
})();
```
Rules: use `Q.React`/`Q.h` (React 18, same-origin, no JSX). `Q.request` can ONLY call `/a/{slug}/tools/*` (the bridge) — it cannot reach `/v1/*` or other apps. Public bundles use plain DOM + inline styles (the rich `__QorvenUI` component set is internal-only). Keep public pages minimal and untrusted-input-safe.

## 3. Tool scripts — same as internal
Public tools are ordinary tool scripts (args via stdin JSON, `$QORVEN_DB_DSN` for the DB). They run in an isolated subprocess with NO secret inheritance. Validate input — these run on untrusted public data.

## 4. Publish + reach
- Admin publishes: `POST /v1/apps/{id}/publish {"external_enabled":true}` (admin-only), or the Apps UI toggle.
- Admin starts a tunnel (Settings → Network → Internet exposure).
- Public surface: `https://<tunnel-host>/a/{slug}` (page) and `/a/{slug}/tools/{name}` (bridge). Only `public:true` pages/tools are reachable; everything else 404s. The admin API is never on the public surface.

## 5. Admin view
The non-public pages (e.g. `admin`) render normally in the authed internal host at `/apps/{slug}/admin` and read the same table the public bridge wrote to. Build both in one app: public page submits, admin page manages.
