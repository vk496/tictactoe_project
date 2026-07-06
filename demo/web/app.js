const $ = (sel) => document.querySelector(sel);

const state = {
  gameId: null,
  workerAddr: null,
  poll: null,
};

function defaultLobby() {
  const fromQuery = new URLSearchParams(location.search).get("lobby");
  return fromQuery || location.origin;
}

async function rpc(base, method, body) {
  const res = await fetch(`${base}/${method}`, {
    method: "POST",
    headers: { "Content-Type": "application/json", "Connect-Protocol-Version": "1" },
    body: JSON.stringify(body),
  });
  const text = await res.text();
  let data = {};
  try {
    data = text ? JSON.parse(text) : {};
  } catch {
    data = {};
  }
  if (!res.ok) {
    const err = new Error(data.message || `${res.status} ${res.statusText}`);
    err.status = res.status;
    throw err;
  }
  return data;
}

function showError(e) {
  if (e && e.status === 410) {
    stopPolling();
    setStatus("This game is no longer available. Start a new game.");
    return;
  }
  setStatus(e.message);
}

function userId() {
  return $("#userId").value.trim();
}

function lobby() {
  return $("#lobby").value.trim().replace(/\/$/, "");
}

function setStatus(msg) {
  $("#status").textContent = msg;
}

function stopPolling() {
  if (state.poll) {
    clearInterval(state.poll);
    state.poll = null;
  }
}

function requireName() {
  if (!userId()) {
    setStatus("Please enter your name first.");
    return false;
  }
  return true;
}

function positiveInt(sel) {
  const n = parseInt($(sel).value, 10);
  return Number.isInteger(n) && n > 0 ? n : 0;
}

async function newGame() {
  if (!requireName()) return;
  stopPolling();
  try {
    const r = await rpc(lobby(), "tictactoe.v1.Lobby/CreateGame", {
      userId: userId(),
      boardSize: positiveInt("#boardSize"),
      winLength: positiveInt("#winLength"),
    });
    state.gameId = r.gameId;
    state.workerAddr = null;
    $("#board").innerHTML = "";
    setStatus("Waiting for an opponent to join…");
    state.poll = setInterval(waitForAssignment, 800);
    refreshPending();
  } catch (e) {
    showError(e);
  }
}

async function waitForAssignment() {
  try {
    const r = await rpc(lobby(), "tictactoe.v1.Lobby/GetGame", { gameId: state.gameId });
    if (r.status === "GAME_STATUS_ASSIGNED" && r.workerAddr) {
      state.workerAddr = r.workerAddr;
      startBoardPolling();
    } else if (r.status === "GAME_STATUS_ABORTED") {
      stopPolling();
      setStatus("Game aborted — the worker was lost.");
    }
  } catch (e) {
    showError(e);
  }
}

async function refreshPending() {
  try {
    const r = await rpc(lobby(), "tictactoe.v1.Lobby/ListPendingGames", {});
    renderPending(r.games || []);
  } catch (e) {
    showError(e);
  }
}

// Lobby-wide views, refreshed on a timer so everyone sees the same thing.
function refreshLobby() {
  refreshPending();
  refreshActive();
  refreshLeaderboard();
}

async function refreshActive() {
  try {
    const r = await rpc(lobby(), "tictactoe.v1.Lobby/ListActiveGames", {});
    const ul = $("#active");
    ul.innerHTML = "";
    const games = r.games || [];
    if (!games.length) {
      ul.innerHTML = `<li class="empty">No games in progress.</li>`;
      return;
    }
    for (const g of games) {
      const size = g.boardSize || 3;
      const li = document.createElement("li");
      li.textContent = `${g.playerX} vs ${g.playerO} · ${size}×${size}`;
      ul.append(li);
    }
  } catch {
    /* best-effort */
  }
}

async function refreshLeaderboard() {
  try {
    const r = await rpc(lobby(), "tictactoe.v1.Lobby/Leaderboard", { limit: 10 });
    const ol = $("#leaderboard");
    ol.innerHTML = "";
    const entries = r.entries || [];
    if (!entries.length) {
      ol.innerHTML = `<li class="empty">No games played yet.</li>`;
      return;
    }
    for (const e of entries) {
      const li = document.createElement("li");
      const who = document.createElement("span");
      who.textContent = e.userId;
      const record = document.createElement("span");
      record.textContent = `${e.wins || 0}W · ${e.losses || 0}L · ${e.draws || 0}D`;
      li.append(who, record);
      ol.append(li);
    }
  } catch {
    /* best-effort */
  }
}

function renderPending(games) {
  const ul = $("#pending");
  ul.innerHTML = "";
  if (games.length === 0) {
    ul.innerHTML = `<li class="empty">No games waiting.</li>`;
    return;
  }
  for (const g of games) {
    const li = document.createElement("li");
    const label = document.createElement("span");
    const size = g.boardSize || 3;
    const win = g.winLength || 3;
    label.textContent = `${g.creatorId} · ${size}×${size}, ${win} to win`;
    const btn = document.createElement("button");
    btn.textContent = "Join";
    btn.disabled = g.creatorId === userId();
    btn.onclick = () => joinGame(g.gameId);
    li.append(label, btn);
    ul.append(li);
  }
}

async function joinGame(gameId) {
  if (!requireName()) return;
  stopPolling();
  try {
    await rpc(lobby(), "tictactoe.v1.Lobby/JoinGame", { gameId, userId: userId() });
    state.gameId = gameId;
    state.workerAddr = null;
    $("#board").innerHTML = "";
    setStatus("Starting game…");
    state.poll = setInterval(waitForAssignment, 800);
  } catch (e) {
    showError(e);
  }
}

function startBoardPolling() {
  stopPolling();
  refreshBoard();
  state.poll = setInterval(refreshBoard, 800);
}

async function refreshBoard() {
  try {
    const v = await rpc(state.workerAddr, "tictactoe.v1.Worker/GetBoard", { gameId: state.gameId });
    renderBoard(v);
  } catch (e) {
    showError(e);
  }
}

function renderBoard(v) {
  const finished = v.status === "GAME_STATUS_WON" || v.status === "GAME_STATUS_DRAWN";
  if (finished) {
    stopPolling();
    if (v.status === "GAME_STATUS_DRAWN") {
      setStatus("It's a draw.");
    } else {
      setStatus(v.winnerId === userId() ? "You win! 🎉" : `${v.winnerId} wins.`);
    }
  } else {
    setStatus(v.turnUserId === userId() ? "Your turn." : `Waiting for ${v.turnUserId}…`);
  }
  drawGrid(v, finished);
  refreshStats();
}

function drawGrid(v, finished) {
  const board = $("#board");
  board.style.setProperty("--size", v.boardSize);
  board.innerHTML = "";
  const myTurn = v.turnUserId === userId();
  (v.rows || []).forEach((row, r) => {
    (row.cells || []).forEach((mark, c) => {
      const cell = document.createElement("button");
      cell.className = "cell";
      if (mark === "MARK_X") { cell.textContent = "X"; cell.classList.add("x"); }
      else if (mark === "MARK_O") { cell.textContent = "O"; cell.classList.add("o"); }
      const empty = mark === "MARK_EMPTY" || mark === undefined;
      cell.disabled = finished || !myTurn || !empty;
      cell.onclick = () => move(r, c);
      board.append(cell);
    });
  });
}

async function move(row, col) {
  try {
    const v = await rpc(state.workerAddr, "tictactoe.v1.Worker/MakeMove", {
      gameId: state.gameId,
      userId: userId(),
      row,
      col,
    });
    renderBoard(v);
  } catch (e) {
    showError(e);
  }
}

async function refreshStats() {
  if (!userId()) return;
  try {
    const s = await rpc(lobby(), "tictactoe.v1.Lobby/GetStats", { userId: userId() });
    $("#stats").textContent = `${s.wins || 0} wins · ${s.losses || 0} losses · ${s.draws || 0} draws`;
  } catch {
    /* stats are best-effort */
  }
}

$("#lobby").value = defaultLobby();
// The slides and dashboards each run on their own port (same host as the game).
const onHost = (port, path = "") => `${location.protocol}//${location.hostname}:${port}${path}`;
$("#slides-link").href = onHost(8100);
$("#link-slides").href = onHost(8100);
$("#link-rabbitmq").href = onHost(8999);
$("#link-traefik").href = onHost(8090, "/dashboard/");
$("#newGame").onclick = newGame;
$("#refresh").onclick = refreshLobby;
$("#refresh").classList.add("secondary");
refreshLobby();
setInterval(refreshLobby, 2000);
