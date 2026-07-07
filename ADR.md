# Architecture Decision Records

Short records of the decisions that shape this backend, and why. They describe
intent, not implementation.

## 1. Split the coordination path from the gameplay path
Coordinating a match (create, find, join, report a result) happens a handful of
times per game; playing it (making moves) is continuous and latency-sensitive.

**Decision:** coordination travels over a message broker between a single
authority and a pool of game workers; moves go straight to the worker that holds
the game. The authority is never on the move path.

**Consequences:** the authority does not grow with move volume and never becomes
the bottleneck; games spread across workers without a bespoke load balancer; the
cost is running a broker.

## 2. Keep all state in memory; bind a game to one worker
The assignment calls for in-memory state, no database.

**Decision:** a worker holds its live games in memory and the authority holds
matchmaking and standings in memory; each game belongs to the worker that first
picked it up.

**Consequences:** no persistence — if a worker is lost its in-progress games are
lost and the players are asked to start again; scaling the authority out later
means sharding it or moving its state to a shared store.

## 3. Board size and win length are chosen per game, with a 3×3 floor
The game must support a configurable board size and win length.

**Decision:** each game sets its own board size and win length when it is created,
falling back to a server default. A board is at least 3×3 — a 1×1 or 2×2 board is
not a real game — and the win length may not exceed the board.

**Consequences:** different games can run different sizes at once, and obviously
unplayable configurations are refused when the game is created.

## 4. One place owns the rules of a game
Turn-based play must stay consistent even under concurrent moves.

**Decision:** a single guardian applies every move and is the only thing that can
change a game. It refuses a move that is out of turn, from a non-player, onto a
taken cell, off the board, or after the game has ended, and it decides win or draw.

**Consequences:** an illegal move can never be committed and the outcome is
unambiguous.

## 5. A player is in at most one game, and idle games are cleaned up
**Decision:** a player may hold only one unfinished game at a time; a game left
idle for too long is abandoned and its players are freed.

**Consequences:** no runaway backlog of half-open games, and a player who walks
away is not blocked forever.

## 6. Joining a game is decided by one authority
**Decision:** when several players race to join the same waiting game, one
authority decides — exactly one gets in and the others are told it is taken.

**Consequences:** no double-joins or split-brain. The trade-off is that this piece
of coordination is centralized, which decision 1 makes affordable.

## 7. A move is authorized by an unforgeable, expiring capability
**Decision:** when a game is placed on a worker, the client is handed a signed,
time-limited token that names that worker. The edge honours a move only if the
token verifies, and routes it only to the worker the token names.

**Consequences:** a client cannot aim traffic at an arbitrary host or reach a game
it was not assigned — with no user accounts or sessions to manage.

## 8. A worker holds a bounded number of games
**Decision:** each worker accepts at most a fixed number of games (200 by
default). When it is full it refuses new ones, and the broker offers them to
another worker instead. Finished games are kept only briefly and long-idle
(abandoned) games are dropped, so capacity frees up over time.

**Consequences:** load spreads across the pool instead of piling onto one worker,
and a worker's memory stays bounded; if every worker is full, new games wait in
line until one has room (the signal to add workers).
