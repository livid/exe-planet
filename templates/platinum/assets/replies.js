// Replies from the exe hub, under a post the site announced there
// (exe-planet's PLAN.md, Replies from the hub).
//
// The Replies window frames the hub's replies page for this post: the
// hub draws the rows, keeps them live and in the reader's language, and
// says how tall they are, so the frame never scrolls on its own. The
// Reply window is here, at the top of this page, because a Solana wallet
// reaches only a page's top level, never a frame inside it. It is the
// hub's own composer, copied: a hub account is a raw ed25519 key, and so
// is a Solana address, so the wallet's own key signs each reply —
// "exe-hub:v1\n" and the envelope, one popup a reply — and the hub's gate
// checks that same address holds its token. There is no session and no
// cookie; the page remembers which wallet signed in (its name and
// address, nothing secret) and asks it again, silently, on the next
// visit. Only ever a message signature, never a transaction. The code
// is this template's, fixed with each build; the hub serves none of it.
(() => {
  const box = document.getElementById("compose"), win = document.getElementById("replies");
  if (!box || !win) return;
  const HUB = box.dataset.hub, ROOT = box.dataset.root;
  const frame = win.querySelector("iframe.thread");
  const q = s => box.querySelector(s);
  const root = document.documentElement;
  const KEY = "exe-hub-wallet", DRAFT = "exe-hub-draft:" + ROOT, PREFIX = "exe-hub:v1\n", MAX_TEXT = 8192;
  const note = q(".note"), signin = q(".signin"), picker = q(".picker"), pickOff = q(".pick-off");
  const text = q(".text"), send = q(".send"), status = q(".status");
  const noteText = note.textContent;
  const enc = new TextEncoder();
  const bytes = s => enc.encode(s).length;
  const short = a => a.length > 10 ? a.slice(0, 4) + "…" + a.slice(-4) : a;
  const b64 = u => { let s = ""; for (let i = 0; i < u.length; i += 0x8000) s += String.fromCharCode(...u.subarray(i, i + 0x8000)); return btoa(s); };
  const same = (a, b) => a.length === b.length && Array.prototype.every.call(a, (x, i) => x === b[i]);
  const store = (k, v) => { try { v == null ? localStorage.removeItem(k) : localStorage.setItem(k, v); } catch (e) {} };
  const stored = k => { try { return localStorage.getItem(k); } catch (e) { return null; } };
  const remember = v => store(KEY, v ? JSON.stringify(v) : null);
  const recall = () => { try { return JSON.parse(stored(KEY) || "null"); } catch (e) { return null; } };
  const declined = e => e && (e.code === 4001 || /reject|declin|cancel|denied/i.test(e.message || ""));
  const hubJSON = async (path, opts) => {
    const r = await fetch(HUB + path, Object.assign({ cache: "no-store", credentials: "omit" }, opts));
    const out = await r.json().catch(() => ({}));
    return { r, out };
  };

  // ---- the frame: its height, and the reply it aims this window at ----
  // A page shown sandboxed (the Planet app's page column) is an opaque
  // origin, and so is every frame inside it: there the hub's frame says
  // "null". The message must still come from this page's own frame.
  let me = null;
  const boxed = self.origin === "null";
  const toFrame = m => { try { frame.contentWindow.postMessage(m, boxed ? "*" : HUB); } catch (e) {} };
  const tellFrame = () => toFrame({ hub: "signed-in", on: !!me });
  addEventListener("message", e => {
    const d = e.data;
    if (e.source !== frame.contentWindow || (e.origin !== HUB && !(boxed && e.origin === "null")) || !d || typeof d !== "object") return;
    if (d.hub === "height" && typeof d.h === "number" && d.h > 0) frame.style.height = Math.ceil(d.h) + "px";
    else if (d.hub === "ready") tellFrame();
    else if (d.hub === "aim" && me && typeof d.id === "string" && /^[0-9a-f]{64}$/.test(d.id)) aim(d);
  });

  // ---- wallets: the Wallet Standard's two-way handshake ----
  const wallets = [];
  let onWallet = () => {};
  const registry = { register(...ws) {
    for (const w of ws) if (w && w.features && w.features["standard:connect"] && w.features["solana:signMessage"] && !wallets.includes(w)) wallets.push(w);
    if (wallets.length) root.classList.add("solana");
    onWallet();
    return () => {};
  } };
  addEventListener("wallet-standard:register-wallet", e => { try { e.detail(registry); } catch (err) {} });
  dispatchEvent(new CustomEvent("wallet-standard:app-ready", { detail: registry }));
  const legacy = () => {
    const p = window.solana;
    return p && typeof p.connect === "function" && typeof p.signMessage === "function"
      ? { name: p.isPhantom ? "Phantom" : "Solana wallet", legacy: p } : null;
  };
  if (legacy()) root.classList.add("solana");
  // wallets announce themselves as the page starts; give them a moment
  const found = new Promise(res => setTimeout(res, 300)).then(() => wallets.length ? wallets : [legacy()].filter(Boolean));

  // connect → { wallet, name, address, pub, sign(message bytes) → signature bytes }
  const solanaAccount = accounts => (accounts || []).find(a => (a.chains || []).some(c => c.startsWith("solana:"))) || (accounts || [])[0];
  async function connect(w, silent) {
    if (w.legacy) {
      const p = w.legacy;
      const r = await p.connect(silent ? { onlyIfTrusted: true } : undefined);
      const pk = (r && r.publicKey) || p.publicKey;
      if (!pk) throw new Error("no account");
      return { wallet: w, name: w.name, address: pk.toBase58(), pub: new Uint8Array(pk.toBytes()),
        sign: async m => { const r = await p.signMessage(m, "utf8"); return new Uint8Array(r.signature || r); } };
    }
    const { accounts } = await w.features["standard:connect"].connect(silent ? { silent: true } : undefined);
    const a = solanaAccount(accounts);
    if (!a) throw new Error("no account");
    return { wallet: w, name: w.name, address: a.address, pub: new Uint8Array(a.publicKey),
      sign: async m => {
        const [out] = await w.features["solana:signMessage"].signMessage({ account: a, message: m });
        if (out.signedMessage && !same(out.signedMessage, m)) throw new Error("changed");
        return new Uint8Array(out.signature);
      } };
  }

  // ---- state ----
  let verdict = null, until = 0, busy = false, flash = "", timer = 0;
  let target = null; // the reply this window answers instead of the post
  let saving = 0;    // the draft's save, debounced behind the typing
  const watched = new WeakSet();

  const need = v => (v.mints || []).map(m => (m.raw ? m.amount + " raw units" : m.amount + " tokens") + " of " + short(m.mint)).join(" or ");
  function line(left) {
    const v = verdict;
    if (!v) return "Checking this address…";
    if (v.banned) return "This key is banned from posting on the hub.";
    if (v.gate === "below") return "This address holds less than " + need(v) + ", so it can read but not reply.";
    if (v.gate === "unavailable") return "The hub can’t check holdings right now. Try again later.";
    if (left > 0) return "You can reply again in " + left + " s.";
    return "Each reply asks your wallet for one signature.";
  }
  // one pass over everything the state decides: the status line, what is
  // enabled, and the cooldown's countdown while it runs
  function render() {
    clearTimeout(timer);
    const v = verdict;
    const left = Math.max(0, Math.ceil((until - Date.now()) / 1000));
    const can = !!v && !v.banned && v.gate !== "below" && v.gate !== "unavailable";
    const t = text.value.trim();
    const over = bytes(t) - MAX_TEXT;
    send.disabled = busy || !can || left > 0 || !t || over > 0;
    status.textContent = over > 0 ? "That is " + over + " bytes past the " + MAX_TEXT + "-byte limit." : flash || line(left);
    if (left > 0 && !flash) timer = setTimeout(render, 1000);
  }
  const tell = s => { flash = s; render(); };
  const done = s => { tell(s); setTimeout(() => { if (flash === s) { flash = ""; render(); } }, 2500); };

  // who is replying: the gate's verdict, and the profile's name and face
  async function check() {
    verdict = null;
    render();
    const author = b64(me.pub);
    try {
      const { r, out } = await hubJSON("/v1/gate?author=" + encodeURIComponent(author));
      if (!r.ok) throw new Error("Could not check this address: " + (out.error || "HTTP " + r.status));
      const p = await hubJSON("/v1/profile/" + out.profile);
      if (!me || b64(me.pub) !== author) return;
      verdict = out;
      until = Date.now() + (out.wait || 0) * 1000;
      const prof = p.r.ok ? p.out : null;
      q(".id").textContent = out.profile;
      q(".id").title = me.address;
      q(".name").textContent = prof && prof.name ? prof.name : "No name yet";
      const av = q(".me-av"), img = av.querySelector("img");
      img.src = prof && prof.avatar ? HUB + "/v1/embed/" + prof.avatar : HUB + "/v1/identicon/" + out.profile + ".svg";
      img.classList.toggle("idn", !(prof && prof.avatar));
      av.href = HUB + "/u/" + out.profile;
      av.hidden = false;
      render();
    } catch (e) {
      if (me) tell(e.message);
    }
  }

  async function start(w, silent) {
    try {
      me = await connect(w, silent);
    } catch (e) {
      me = null;
      signedOut();
      if (!silent) note.textContent = declined(e) ? "Sign-in was declined in the wallet." : "The wallet did not connect: " + (e.message || e);
      return;
    }
    remember({ name: me.name, address: me.address });
    root.classList.add("wallet");
    q(".name").textContent = "…";
    q(".id").textContent = short(me.address);
    q(".me-av").hidden = true;
    flash = "";
    watch(w);
    tellFrame();
    await check();
  }
  function signedOut() {
    verdict = null; flash = ""; until = 0;
    remember(null);
    root.classList.remove("wallet");
    q(".me-av").hidden = true;
    tellFrame();
  }
  // the wallet switched accounts or forgot this site
  function watch(w) {
    const ev = !w.legacy && w.features["standard:events"];
    if (!ev || watched.has(w)) return;
    watched.add(w);
    ev.on("change", ch => {
      if (!me || me.wallet !== w || !ch || !ch.accounts) return;
      const a = solanaAccount(ch.accounts);
      if (!a) { me = null; signedOut(); }
      else if (a.address !== me.address) start(w, true);
    });
  }

  // ---- one signed reply ----
  async function signed(msg) {
    try {
      return await me.sign(msg);
    } catch (e) {
      throw new Error(e.message === "changed" ? "The wallet changed the message before signing it, so the hub could not verify it."
        : declined(e) ? "You declined in the wallet." : "The wallet could not sign: " + (e.message || e));
    }
  }
  async function sendReply(body) {
    const author = b64(me.pub);
    const s = await hubJSON("/v1/seq?author=" + encodeURIComponent(author));
    if (!s.r.ok) throw new Error(s.out.error || "HTTP " + s.r.status);
    const env = enc.encode(JSON.stringify({ type: "post.create", author, seq: s.out.seq + 1, ts: Date.now(), body }));
    const msg = new Uint8Array(PREFIX.length + env.length);
    msg.set(enc.encode(PREFIX));
    msg.set(env, PREFIX.length);
    tell("Waiting for your wallet…");
    const sig = await signed(msg);
    tell("Sending…");
    const { r, out } = await hubJSON("/v1/msg", { method: "POST", headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ envelope: b64(env), sig: b64(sig) }) });
    if (r.ok) return out.id;
    if (r.status === 429) {
      until = Date.now() + (parseInt(r.headers.get("Retry-After"), 10) || (verdict && verdict.cooldown) || 60) * 1000;
      throw new Error("");
    }
    if (r.status === 403) { await check(); if (verdict && verdict.gate === "below") throw new Error(""); }
    if (r.status === 401) throw new Error("The hub could not verify the signature.");
    if (r.status === 409) throw new Error("Your reply may have landed already; reload to see, or press Reply again.");
    throw new Error(out.error || "HTTP " + r.status);
  }

  // ---- the reply this window answers ----
  // A Reply link under a reply in the frame aims this window at it: a line
  // names it ("Replying to Name — its first words") with a cross that lets
  // it go, so the window answers the post again. The aim belongs to the
  // draft, not to the session: it is saved with the words and comes back
  // with them after a reload, and a sign-out leaves it be. The reply is
  // checked on the hub as Reply is pressed: words written to one reply
  // are never sent anywhere else by themselves, and one deleted meanwhile
  // is said so, the words and the aim kept until the reader lets it go.
  const reRow = q(".re-row"), reQ = q(".re-q"), reClear = q(".re-clear");
  const aimOf = d => d && typeof d === "object" && typeof d.id === "string" && /^[0-9a-f]{64}$/.test(d.id)
    ? { id: d.id, name: String(d.name || "").slice(0, 64), words: String(d.words || "").slice(0, 200) } : null;
  function showTarget() {
    reRow.hidden = !target;
    if (target) reQ.replaceChildren("Replying to ", Object.assign(document.createElement("b"), { textContent: target.name }), target.words ? " — " + target.words : "");
    render();
  }
  function aim(d) {
    target = aimOf(d);
    showTarget();
    saveDraft();
    box.closest(".window").scrollIntoView({ block: "center" });
    text.focus({ preventScroll: true });
  }
  reClear.addEventListener("click", () => { target = null; showTarget(); saveDraft(); text.focus(); });

  // ---- the draft: the words and the reply they answer, kept together ----
  // {v: 1, text, reply_to} under the post's key; a draft an earlier build
  // saved as bare words reads as words answering the post. Words alone
  // are the draft: an aim with nothing written is not kept.
  function saveDraft() {
    clearTimeout(saving);
    store(DRAFT, text.value ? JSON.stringify({ v: 1, text: text.value, reply_to: target }) : null);
  }
  function readDraft() {
    const s = stored(DRAFT);
    if (!s) return null;
    try {
      const j = JSON.parse(s);
      if (j && typeof j === "object" && j.v === 1) return { text: String(j.text || ""), reply_to: aimOf(j.reply_to) };
    } catch (e) {}
    return { text: s, reply_to: null };
  }

  // ---- the controls ----
  signin.addEventListener("click", async () => {
    const ws = await found;
    if (ws.length === 1) return start(ws[0], false);
    if (!ws.length) return;
    const label = Object.assign(document.createElement("span"), { className: "grow", textContent: "Choose a wallet:" });
    picker.replaceChildren(label, ...ws.map(w => {
      const b = document.createElement("button");
      b.type = "button";
      b.className = "btn wbtn";
      if (w.icon) { const i = document.createElement("img"); i.src = w.icon; i.alt = ""; b.append(i); }
      b.append(w.name);
      b.addEventListener("click", () => { picker.hidden = true; pickOff.hidden = false; start(w, false); });
      return b;
    }), Object.assign(document.createElement("button"), { type: "button", className: "btn", textContent: "Cancel",
      onclick: () => { picker.hidden = true; pickOff.hidden = false; } }));
    pickOff.hidden = true;
    picker.hidden = false;
  });
  q(".signout").addEventListener("click", () => {
    const w = me && me.wallet;
    me = null;
    signedOut();
    note.textContent = noteText;
    try {
      if (w && w.legacy && w.legacy.disconnect) w.legacy.disconnect();
      else if (w && w.features["standard:disconnect"]) w.features["standard:disconnect"].disconnect().catch(() => {});
    } catch (e) {}
  });
  text.addEventListener("input", () => {
    flash = "";
    render();
    clearTimeout(saving);
    saving = setTimeout(saveDraft, 400);
  });
  text.addEventListener("keydown", e => { if (e.key === "Enter" && (e.metaKey || e.ctrlKey) && !send.disabled) send.click(); });
  send.addEventListener("click", async () => {
    const t = text.value.trim();
    if (!t || busy || !me) return;
    busy = true;
    render();
    try {
      const to = target ? target.id : ROOT;
      if (target) {
        tell("Checking the reply…");
        const { r } = await hubJSON("/v1/post/" + target.id);
        if (r.status === 404) throw new Error("That reply is gone. Clear it to answer the post instead.");
      }
      await sendReply({ text: t, reply_to: to });
      text.value = "";
      clearTimeout(saving);
      store(DRAFT, null);
      target = null;
      reRow.hidden = true;
      until = Date.now() + ((verdict && verdict.cooldown) || 0) * 1000;
      done("Replied.");
    } catch (e) {
      tell(e.message);
    } finally {
      busy = false;
      render();
    }
  });

  // ---- start: the draft, and no wallet, one signed in before, or none ----
  const draft = readDraft();
  if (draft) {
    text.value = draft.text;
    target = draft.reply_to;
    showTarget();
  }
  const saved = recall();
  found.then(ws => {
    if (!ws.length) {
      note.textContent = "Replying needs a Solana wallet in this browser.";
      signin.disabled = true;
      onWallet = () => { note.textContent = noteText; signin.disabled = false; };
    }
    const w = saved && ws.find(w => w.name === saved.name);
    if (w) start(w, true);
    else if (saved) signedOut();
  });
})();
