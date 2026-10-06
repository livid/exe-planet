// Replies from the exe hub, under a post the site announced there
// (exe-planet's PLAN.md, Replies from the hub). Platinum's replies.js,
// copied, with one change: every word the composer says is in the table
// below, in the site's language (the page's lang, as the template's
// Language setting writes it), the words the page starts with rendered
// by the template's w_*.html modules in the same four languages. Fix a
// bug in one copy, fix it in the other.
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
// address, nothing secret) and on the next visit is signed in as that
// address, the wallet asked again only at the next reply to sign. Only
// ever a message signature, never a transaction. The code is this
// template's, fixed with each build; the hub serves none of it. The
// sign-in follows the hub's own pages (exe-hub's PLAN.md, Posting from a
// wallet): a fix to the flow there comes here too.
(() => {
  // ---- the words, in the site's four languages ----
  const WORDS = {
    en: {
      checking: "Checking this address…", banned: "This key is banned from posting on the hub.",
      below: need => "This address holds less than " + need + ", so it can read but not reply.",
      amount: (n, mint, raw) => n + (raw ? " raw units" : " tokens") + " of " + mint, or: " or ",
      unavailable: "The hub can’t check holdings right now. Try again later.",
      wait: s => "You can reply again in " + s + " s.", ready: "Each reply asks your wallet for one signature.",
      over: (n, max) => "That is " + n + " bytes past the " + max + "-byte limit.",
      nocheck: "Could not check this address: ", noname: "No name yet",
      signinDeclined: "Sign-in was declined in the wallet.", noconnect: "The wallet did not connect: ",
      changed: "The wallet changed the message before signing it, so the hub could not verify it.",
      declined: "You declined in the wallet.", nosign: "The wallet could not sign: ",
      waiting: "Waiting for your wallet…", sending: "Sending…", badsig: "The hub could not verify the signature.",
      maybe: "Your reply may have landed already; reload to see, or press Reply again.",
      replyingTo: (b, words) => ["Replying to ", b, words ? " — " + words : ""],
      choose: "Choose a wallet:", cancel: "Cancel", clear: "Answer the post instead",
      checkingReply: "Checking the reply…", gone: "That reply is gone. Clear it to answer the post instead.",
      replied: "Replied.", nowallet: "Replying needs a Solana wallet in this browser.",
      switched: "Your wallet is on another account now, shown above. Nothing was signed; try again as that account, or switch back in the wallet.",
      signinTitle: host => "Sign in to " + host,
      signinStmt: "You signed out here before. This signature only shows it is you again: it is not sent anywhere, and it is not a transaction.",
    },
    "zh-Hans": {
      checking: "正在查询这个地址……", banned: "这把密钥已被禁止在 hub 发帖。",
      below: need => "这个地址持有的少于 " + need + "，只能阅读，不能回复。",
      amount: (n, mint, raw) => n + (raw ? " 个最小单位的 " : " 枚 ") + mint + (raw ? "" : " 代币"), or: " 或 ",
      unavailable: "hub 暂时无法查询持仓，请稍后再试。",
      wait: s => s + " 秒后可以再次回复。", ready: "每条回复需要钱包签名一次。",
      over: (n, max) => "已超出 " + max + " 字节的上限 " + n + " 字节。",
      nocheck: "无法查询这个地址：", noname: "尚未设置名字",
      signinDeclined: "已在钱包中取消登录。", noconnect: "钱包没有连接：",
      changed: "钱包在签名前改动了消息，hub 无法验证。",
      declined: "你在钱包中取消了。", nosign: "钱包无法签名：",
      waiting: "等待钱包确认……", sending: "正在发送……", badsig: "hub 无法验证这个签名。",
      maybe: "你的回复可能已经发出；刷新页面查看，或再按一次回复。",
      replyingTo: (b, words) => ["回复 ", b, words ? "——" + words : ""],
      choose: "选择钱包：", cancel: "取消", clear: "改为回复文章",
      checkingReply: "正在核对那条回复……", gone: "那条回复已被删除。清除后可直接回复文章。",
      replied: "已回复。", nowallet: "回复需要这个浏览器里有 Solana 钱包。",
      switched: "钱包现在是另一个账户，已显示在上面。什么都没有签名；用这个账户再试一次，或在钱包里切换回去。",
      signinTitle: host => "登录 " + host,
      signinStmt: "你之前在这里退出过登录。这个签名只是确认又是你本人：它不会被发送到任何地方，也不是交易。",
    },
    "zh-Hant": {
      checking: "正在查詢這個地址……", banned: "這把金鑰已被禁止在 hub 發文。",
      below: need => "這個地址持有的少於 " + need + "，只能閱讀，不能回覆。",
      amount: (n, mint, raw) => n + (raw ? " 個最小單位的 " : " 枚 ") + mint + (raw ? "" : " 代幣"), or: " 或 ",
      unavailable: "hub 暫時無法查詢持倉，請稍後再試。",
      wait: s => s + " 秒後可以再次回覆。", ready: "每則回覆需要錢包簽名一次。",
      over: (n, max) => "已超出 " + max + " 位元組的上限 " + n + " 位元組。",
      nocheck: "無法查詢這個地址：", noname: "尚未設定名稱",
      signinDeclined: "已在錢包中取消登入。", noconnect: "錢包沒有連線：",
      changed: "錢包在簽名前改動了訊息，hub 無法驗證。",
      declined: "你在錢包中取消了。", nosign: "錢包無法簽名：",
      waiting: "等待錢包確認……", sending: "正在傳送……", badsig: "hub 無法驗證這個簽名。",
      maybe: "你的回覆可能已經送出；重新整理頁面查看，或再按一次回覆。",
      replyingTo: (b, words) => ["回覆 ", b, words ? "——" + words : ""],
      choose: "選擇錢包：", cancel: "取消", clear: "改為回覆文章",
      checkingReply: "正在核對那則回覆……", gone: "那則回覆已被刪除。清除後可直接回覆文章。",
      replied: "已回覆。", nowallet: "回覆需要這個瀏覽器裡有 Solana 錢包。",
      switched: "錢包現在是另一個帳戶，已顯示在上面。什麼都沒有簽名；用這個帳戶再試一次，或在錢包裡切換回去。",
      signinTitle: host => "登入 " + host,
      signinStmt: "你之前在這裡登出過。這個簽名只是確認又是你本人：它不會被傳送到任何地方，也不是交易。",
    },
    ja: {
      checking: "このアドレスを確認しています…", banned: "この鍵は hub への投稿を禁止されています。",
      below: need => "このアドレスの保有量が " + need + " に満たないため、読むことはできますが返信はできません。",
      amount: (n, mint, raw) => mint + (raw ? " の最小単位 " + n : " トークン " + n + " 枚"), or: " または ",
      unavailable: "hub は現在、保有量を確認できません。しばらくしてからお試しください。",
      wait: s => "あと " + s + " 秒で再び返信できます。", ready: "返信のたびにウォレットの署名が 1 回必要です。",
      over: (n, max) => "上限の " + max + " バイトを " + n + " バイト超えています。",
      nocheck: "このアドレスを確認できませんでした：", noname: "名前はまだありません",
      signinDeclined: "ウォレットでサインインが拒否されました。", noconnect: "ウォレットに接続できませんでした：",
      changed: "ウォレットが署名前にメッセージを変更したため、hub で検証できません。",
      declined: "ウォレットで拒否されました。", nosign: "ウォレットで署名できませんでした：",
      waiting: "ウォレットの確認を待っています…", sending: "送信しています…", badsig: "hub で署名を検証できませんでした。",
      maybe: "返信はすでに届いているかもしれません。再読み込みして確かめるか、もう一度「返信」を押してください。",
      replyingTo: (b, words) => [b, " さんへの返信", words ? " — " + words : ""],
      choose: "ウォレットを選択：", cancel: "キャンセル", clear: "記事に返信する",
      checkingReply: "返信先を確認しています…", gone: "返信先は削除されました。クリアすると記事に返信できます。",
      replied: "返信しました。", nowallet: "返信するには、このブラウザに Solana ウォレットが必要です。",
      switched: "ウォレットは別のアカウントに切り替わっています（上に表示）。何も署名していません。そのアカウントでもう一度試すか、ウォレットで元に戻してください。",
      signinTitle: host => host + " にサインイン",
      signinStmt: "以前ここでサインアウトしました。この署名はあなた本人であることを確かめるだけのもので、どこにも送信されず、トランザクションでもありません。",
    },
  };

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
  const W = WORDS[document.documentElement.lang] || WORDS["zh-Hans"];
  q(".re-clear").setAttribute("aria-label", W.clear);
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

  // connect → { wallet, name, address, pub, sign(message bytes) → signature bytes, live }
  // live: the wallet itself handed this account over on this visit
  const solanaAccount = accounts => (accounts || []).find(a => (a.chains || []).some(c => c.startsWith("solana:"))) || (accounts || [])[0];
  async function connect(w, silent) {
    if (w.legacy) {
      const p = w.legacy;
      const r = await p.connect(silent ? { onlyIfTrusted: true } : undefined);
      const pk = (r && r.publicKey) || p.publicKey;
      if (!pk) throw new Error("no account");
      return { wallet: w, name: w.name, address: pk.toBase58(), pub: new Uint8Array(pk.toBytes()), live: true,
        sign: async m => { const r = await p.signMessage(m, "utf8"); return new Uint8Array(r.signature || r); } };
    }
    const { accounts } = await w.features["standard:connect"].connect(silent ? { silent: true } : undefined);
    const a = solanaAccount(accounts);
    if (!a) throw new Error("no account");
    return bind(w, a);
  }
  const bind = (w, a) => ({ wallet: w, name: w.name, address: a.address, pub: new Uint8Array(a.publicKey), live: true,
    sign: async m => {
      const [out] = await w.features["solana:signMessage"].signMessage({ account: a, message: m });
      if (out.signedMessage && !same(out.signedMessage, m)) throw new Error("changed");
      return new Uint8Array(out.signature);
    } });
  // A wallet that signed in on an earlier visit is asked nothing while
  // the page is read: a silent connect is a hint a wallet may not take
  // (Glow in Safari put up its connect prompt now and then on a page
  // opened only to read). The address the page remembers is the account:
  // the gate is checked and the name drawn from it with no wallet at all.
  // The wallet is first asked at the first reply to sign — silently, then
  // aloud if that brings nothing, a press being when a prompt is looked
  // for — and a wallet that answers with another account signs nothing:
  // the window turns to that account and says so, and the next press is
  // that account's.
  const B58 = "123456789ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz";
  function unb58(s) {
    let n = 0n;
    for (const c of s) { const i = B58.indexOf(c); if (i < 0) return null; n = n * 58n + BigInt(i); }
    const out = [];
    for (; n > 0n; n >>= 8n) out.unshift(Number(n & 255n));
    for (const c of s) { if (c !== "1") break; out.unshift(0); }
    return new Uint8Array(out);
  }
  function resume(w, saved) {
    const pub = typeof saved.address === "string" && unb58(saved.address);
    if (!pub || pub.length !== 32) return null;
    let live = null;
    const m = { wallet: w, name: w.name, address: saved.address, pub, live: false,
      sign: async msg => {
        if (!live) {
          const got = await connect(w, true).catch(() => null) || await connect(w, false);
          if (got.address !== m.address) throw Object.assign(new Error("switched"), { account: got });
          live = got;
          m.live = true;
        }
        return live.sign(msg);
      } };
    return m;
  }

  // ---- state ----
  let verdict = null, until = 0, busy = false, flash = "", timer = 0;
  let target = null; // the reply this window answers instead of the post
  let saving = 0;    // the draft's save, debounced behind the typing
  const watched = new WeakSet();

  const need = v => (v.mints || []).map(m => W.amount(m.amount, short(m.mint), m.raw)).join(W.or);
  function line(left) {
    const v = verdict;
    if (!v) return W.checking;
    if (v.banned) return W.banned;
    if (v.gate === "below") return W.below(need(v));
    if (v.gate === "unavailable") return W.unavailable;
    if (left > 0) return W.wait(left);
    return W.ready;
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
    status.textContent = over > 0 ? W.over(over, MAX_TEXT) : flash || line(left);
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
      if (!r.ok) throw new Error(W.nocheck + (out.error || "HTTP " + r.status));
      const p = await hubJSON("/v1/profile/" + out.profile);
      if (!me || b64(me.pub) !== author) return;
      verdict = out;
      until = Date.now() + (out.wait || 0) * 1000;
      const prof = p.r.ok ? p.out : null;
      // with the hub id, the profile's name and picture, which the next
      // visit's who line is drawn from as the page is parsed (the script
      // after it in modules/replies.html), so it shows them from the
      // first paint
      remember({ name: me.name, address: me.address, id: out.profile,
        pname: prof && prof.name || "", ava: prof && prof.avatar || "" });
      q(".id").textContent = out.profile;
      q(".id").title = me.address;
      q(".name").textContent = prof && prof.name ? prof.name : W.noname;
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

  // Sign in with Solana, pressed. A wallet that still trusts this site
  // connects without a word, even after Sign Out — which forgets the
  // wallet here and asks it to disconnect, but only the wallet keeps its
  // list of trusted sites. So the first Sign in after a Sign Out pressed
  // in this browser asks for a signature too, the one request every
  // wallet puts to its owner: with Sign In With Solana, one prompt for
  // both; otherwise a connect, then a message that says what it is.
  // Never "exe-hub:v1", so it can be nothing the hub would take, and it
  // is kept nowhere.
  const OUT = "exe-hub-signed-out";
  const wasOut = () => !!stored(OUT);
  async function start(w) {
    let got;
    try {
      got = wasOut() ? await confirm(w) : await connect(w, false);
    } catch (e) {
      me = null;
      signedOut();
      note.textContent = declined(e) ? W.signinDeclined : W.noconnect + (e.message || e);
      return;
    }
    store(OUT, null);
    await signedIn(got);
  }
  async function confirm(w) {
    const siws = !w.legacy && w.features["solana:signIn"];
    if (siws) {
      const [out] = await siws.signIn({ domain: location.host, uri: location.origin, version: "1", statement: W.signinStmt, issuedAt: new Date().toISOString() });
      if (!out || !out.account) throw new Error("no account");
      return bind(w, out.account);
    }
    const got = await connect(w, false);
    await got.sign(enc.encode(W.signinTitle(location.host) + "\n\n" + W.signinStmt + "\n\n" + new Date().toISOString()));
    return got;
  }
  async function signedIn(m) {
    me = m;
    // the same wallet as last time keeps the who line the page drew
    // as it was parsed, until the gate's answer redraws it the same or
    // with what changed since; another account starts from its address
    const before = recall(), same = before && before.address === me.address;
    remember(same ? Object.assign(before, { name: me.name }) : { name: me.name, address: me.address });
    root.classList.add("wallet");
    if (!same || !before.id) {
      q(".name").textContent = "…";
      q(".id").textContent = short(me.address);
      q(".me-av").hidden = true;
    }
    flash = "";
    watch(me.wallet);
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
      // a wallet not yet asked on this visit has nothing to take back
      if (!a) { if (me.live) { me = null; signedOut(); } }
      else if (a.address !== me.address) signedIn(bind(w, a));
    });
  }

  // ---- one signed reply ----
  async function signed(msg) {
    try {
      return await me.sign(msg);
    } catch (e) {
      if (e.account) { signedIn(e.account); throw new Error(W.switched); } // a remembered wallet came back on another account
      throw new Error(e.message === "changed" ? W.changed : declined(e) ? W.declined : W.nosign + (e.message || e));
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
    tell(W.waiting);
    const sig = await signed(msg);
    tell(W.sending);
    const { r, out } = await hubJSON("/v1/msg", { method: "POST", headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ envelope: b64(env), sig: b64(sig) }) });
    if (r.ok) return out.id;
    if (r.status === 429) {
      until = Date.now() + (parseInt(r.headers.get("Retry-After"), 10) || (verdict && verdict.cooldown) || 60) * 1000;
      throw new Error("");
    }
    if (r.status === 403) { await check(); if (verdict && verdict.gate === "below") throw new Error(""); }
    if (r.status === 401) throw new Error(W.badsig);
    if (r.status === 409) throw new Error(W.maybe);
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
    if (target) reQ.replaceChildren(...W.replyingTo(Object.assign(document.createElement("b"), { textContent: target.name }), target.words));
    render();
  }
  function aim(d) {
    target = aimOf(d);
    showTarget();
    saveDraft();
    box.scrollIntoView({ block: "center" });
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
    if (ws.length === 1) return start(ws[0]);
    if (!ws.length) return;
    const label = Object.assign(document.createElement("span"), { className: "grow", textContent: W.choose });
    picker.replaceChildren(label, ...ws.map(w => {
      const b = document.createElement("button");
      b.type = "button";
      b.className = "btn wbtn";
      if (w.icon) { const i = document.createElement("img"); i.src = w.icon; i.alt = ""; b.append(i); }
      b.append(w.name);
      b.addEventListener("click", () => { picker.hidden = true; pickOff.hidden = false; start(w); });
      return b;
    }), Object.assign(document.createElement("button"), { type: "button", className: "btn", textContent: W.cancel,
      onclick: () => { picker.hidden = true; pickOff.hidden = false; } }));
    pickOff.hidden = true;
    picker.hidden = false;
  });
  q(".signout").addEventListener("click", () => {
    const w = me && me.wallet;
    me = null;
    signedOut();
    store(OUT, "1"); // the next Sign in asks for a signature (start)
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
        tell(W.checkingReply);
        const { r } = await hubJSON("/v1/post/" + target.id);
        if (r.status === 404) throw new Error(W.gone);
      }
      await sendReply({ text: t, reply_to: to });
      text.value = "";
      clearTimeout(saving);
      store(DRAFT, null);
      target = null;
      reRow.hidden = true;
      until = Date.now() + ((verdict && verdict.cooldown) || 0) * 1000;
      done(W.replied);
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
      note.textContent = W.nowallet;
      signin.disabled = true;
      onWallet = () => { note.textContent = noteText; signin.disabled = false; };
    }
    const w = saved && ws.find(w => w.name === saved.name), m = w && resume(w, saved);
    if (m) signedIn(m); // nothing asked of the wallet until a reply is signed
    else if (saved) signedOut();
  });
})();
