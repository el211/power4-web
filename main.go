package main

import (
	crand "crypto/rand"
	_ "embed"
	"encoding/hex"
	"fmt"
	"html/template"
	"log"
	mrand "math/rand"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

// --- Embedded templates & CSS ---
//
//go:embed templates/base.html
var baseTpl string

//go:embed templates/start.html
var startTpl string

//go:embed templates/game.html
var gameTpl string

//go:embed templates/result.html
var resultTpl string

//go:embed static/style.css
var cssBytes []byte

const (
	cellEmpty = byte(0)
	cellR     = byte('R')
	cellY     = byte('Y')
	cellBlk   = byte('X') // immobile block
)

type Game struct {
	Rows, Cols int
	Grid       [][]byte
	Winning    [][]bool
	Current    byte
	Player1    string
	Player2    string
	Scores     struct{ R, Y int }
	Message    string
	GameOver   bool
	Turns      int
	GravityUp  bool
	Mode       string // "local" | "ai" | "online"
	CreatedAt  time.Time
	Difficulty string

	// online
	LobbyCode string
	ThisIsRed bool // viewer flag for online page

	// NEW: who placed the most recent piece ('R' or 'Y')
	LastPlayed byte
}

type ChatMessage struct {
	ID   int64
	When time.Time
	Side string // "R" ou "Y"
	Name string // affiché (P1/P2)
	Text string // contenu
}

type lobby struct {
	Game       *Game
	UpdatedAt  time.Time
	HasRed     bool
	HasYellow  bool
	Chat       []ChatMessage
	NextChatID int64
}

type server struct {
	tpl      *template.Template
	mu       sync.Mutex
	sessions map[string]*Game
	lobbies  map[string]*lobby
}

func main() {
	mrand.Seed(time.Now().UnixNano())

	s := &server{
		tpl:      template.Must(template.New("base").Parse(baseTpl + startTpl + gameTpl + resultTpl)),
		sessions: make(map[string]*Game),
		lobbies:  make(map[string]*lobby),
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/", s.handleStart)
	mux.HandleFunc("/start", s.handleStartPost)
	mux.HandleFunc("/game", s.handleGame)
	mux.HandleFunc("/play", s.handlePlay)
	mux.HandleFunc("/replay", s.handleReplay)
	mux.HandleFunc("/reset", s.handleReset)
	mux.HandleFunc("/result", s.handleResult)

	// Online (MVP)
	mux.HandleFunc("/online/create", s.handleOnlineCreate)
	mux.HandleFunc("/online/join", s.handleOnlineJoin)
	mux.HandleFunc("/online/wait", s.handleOnlineWait)
	mux.HandleFunc("/online/state", s.handleOnlineState)
	mux.HandleFunc("/online/play", s.handleOnlinePlay)
	mux.HandleFunc("/chat/post", s.handleChatPost)
	mux.HandleFunc("/chat/feed", s.handleChatFeed)

	// Static
	mux.HandleFunc("/static/style.css", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/css; charset=utf-8")
		_, _ = w.Write(cssBytes)
	})
	// Serve images (e.g., bg-space.jpg) from disk
	mux.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.Dir("static"))))

	// Health
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	})

	port := os.Getenv("SERVER_PORT")
	if port == "" {
		port = "8080"
	}
	srv := &http.Server{
		Addr:         ":" + port,
		Handler:      securityHeaders(mux),
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}
	log.Printf("Power4 BONUS listening on :%s\n", port)
	log.Fatal(srv.ListenAndServe())
}

func (s *server) handleStart(w http.ResponseWriter, r *http.Request) {
	g := s.gameForRequest(w, r, false)
	data := map[string]any{
		"Player1":    g.Player1,
		"Player2":    g.Player2,
		"Difficulty": g.Difficulty,
	}
	s.render(w, "start", data)
}

func (s *server) handleStartPost(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	mode := strings.ToLower(strings.TrimSpace(r.FormValue("mode")))
	oa := strings.ToLower(strings.TrimSpace(r.FormValue("online_action"))) // "create" | "join" | ""

	// If an online button was clicked, force online mode regardless of dropdown
	if mode != "online" && (oa == "create" || oa == "join") {
		mode = "online"
	}
	if mode == "" {
		mode = "local"
	}

	p1 := strings.TrimSpace(r.FormValue("player1"))
	p2 := strings.TrimSpace(r.FormValue("player2"))
	if p1 == "" {
		p1 = "Rouge"
	}
	if p2 == "" {
		p2 = "Jaune"
	}

	diff := strings.ToLower(strings.TrimSpace(r.FormValue("difficulty")))
	if diff == "" {
		diff = "easy"
	}

	rows, cols, blocks := configByDifficulty(diff)

	switch mode {
	case "local":
		g := s.gameForRequest(w, r, true)
		*g = *newGame(rows, cols, blocks)
		g.Player1, g.Player2 = p1, p2
		g.Difficulty = diff
		g.Mode = "local"
		http.Redirect(w, r, "/game", http.StatusSeeOther)
		return

	case "ai":
		g := s.gameForRequest(w, r, true)
		*g = *newGame(rows, cols, blocks)
		g.Player1, g.Player2 = p1, p2
		g.Difficulty = diff
		g.Mode = "ai"
		http.Redirect(w, r, "/game", http.StatusSeeOther)
		return

	case "online":
		code := strings.ToUpper(strings.TrimSpace(r.FormValue("lobby_code")))

		// If "Join" button pressed OR (big button with a code) => join
		if oa == "join" || (oa == "" && len(code) >= 4) {
			http.Redirect(w, r, "/online/join?code="+code, http.StatusSeeOther)
			return
		}

		// Otherwise => create (auto code generated server-side)
		createURL := "/online/create?rows=" + strconv.Itoa(rows) + "&cols=" + strconv.Itoa(cols) + "&blocks=" + strconv.Itoa(blocks) +
			"&p1=" + urlQueryEscape(p1) + "&p2=" + urlQueryEscape(p2) + "&diff=" + diff
		http.Redirect(w, r, createURL, http.StatusSeeOther)
		return

	default:
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
}

func urlQueryEscape(s string) string {
	// light helper (avoids importing net/url here)
	replacer := strings.NewReplacer(" ", "+")
	return replacer.Replace(s)
}

func (s *server) handleGame(w http.ResponseWriter, r *http.Request) {
	g := s.gameForRequest(w, r, false)
	data := s.viewModel(g)
	s.render(w, "game", data)
}

func (s *server) handlePlay(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Redirect(w, r, "/game", http.StatusSeeOther)
		return
	}
	g := s.gameForRequest(w, r, false)
	if g.GameOver {
		http.Redirect(w, r, "/result", http.StatusSeeOther)
		return
	}

	colStr := r.FormValue("col")
	c, err := strconv.Atoi(colStr)
	if err != nil || c < 0 || c >= g.Cols {
		http.Redirect(w, r, "/game", http.StatusSeeOther)
		return
	}

	row := dropRow(g.Grid, c, g.GravityUp)
	if row == -1 || g.Grid[row][c] != cellEmpty {
		http.Redirect(w, r, "/game", http.StatusSeeOther)
		return
	}
	g.Grid[row][c] = g.Current
	g.LastPlayed = g.Current

	g.Turns++

	// Win / Draw?
	if s.checkResult(g, row, c, g.Current) {
		http.Redirect(w, r, "/result", http.StatusSeeOther)
		return
	}

	// Switch player
	if g.Current == cellR {
		g.Current = cellY
	} else {
		g.Current = cellR
	}

	// Flip gravity every 5 moves
	if g.Turns%5 == 0 {
		g.GravityUp = !g.GravityUp
		g.Message = ""
	}

	// If AI mode and now it's AI's turn, let AI play immediately
	if g.Mode == "ai" && g.Current == cellY && !g.GameOver {
		aiCol := chooseAIMove(g)
		if aiCol >= 0 {
			rowAI := dropRow(g.Grid, aiCol, g.GravityUp)
			if rowAI != -1 && g.Grid[rowAI][aiCol] == cellEmpty {
				g.Grid[rowAI][aiCol] = cellY
				g.LastPlayed = g.Current
				g.Turns++
				if s.checkResult(g, rowAI, aiCol, cellY) {
					http.Redirect(w, r, "/result", http.StatusSeeOther)
					return
				}
				// switch back to human
				g.Current = cellR
				if g.Turns%5 == 0 {
					g.GravityUp = !g.GravityUp
					g.Message = ""
				}
			}
		}
	}

	http.Redirect(w, r, "/game", http.StatusSeeOther)
}

func (s *server) handleReplay(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Redirect(w, r, "/game", http.StatusSeeOther)
		return
	}
	g := s.gameForRequest(w, r, false)
	diff := g.Difficulty
	rows, cols, blocks := configByDifficulty(diff)
	scoreR, scoreY := g.Scores.R, g.Scores.Y
	p1, p2 := g.Player1, g.Player2
	*g = *newGame(rows, cols, blocks)
	g.Player1, g.Player2 = p1, p2
	g.Scores.R, g.Scores.Y = scoreR, scoreY
	g.Difficulty = diff
	http.Redirect(w, r, "/game", http.StatusSeeOther)
}

func (s *server) handleReset(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	_ = s.gameForRequest(w, r, true) // reset session
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (s *server) handleResult(w http.ResponseWriter, r *http.Request) {
	g := s.gameForRequest(w, r, false)
	data := s.viewModel(g)
	s.render(w, "result", data)
}

func (s *server) checkResult(g *Game, r, c int, p byte) bool {
	line := winningLine(g.Grid, r, c, p)
	if len(line) >= 4 {
		for _, rc := range line[:4] {
			g.Winning[rc[0]][rc[1]] = true
		}
		g.GameOver = true
		if p == cellR {
			g.Scores.R++
		} else {
			g.Scores.Y++
		}
		g.Message = ""
		return true
	}
	if isDraw(g.Grid) {
		g.GameOver = true
		g.Message = "🤝 Égalité !"
		return true
	}
	return false
}

/*** helpers ***/

func configByDifficulty(d string) (rows, cols, blocks int) {
	switch d {
	case "hard":
		return 7, 8, 7
	case "normal":
		return 6, 9, 5
	default: // easy
		return 6, 7, 3
	}
}

func newGame(rows, cols, blocks int) *Game {
	g := &Game{
		Rows:      rows,
		Cols:      cols,
		Grid:      make([][]byte, rows),
		Winning:   make([][]bool, rows),
		Current:   cellR,
		Mode:      "local",
		CreatedAt: time.Now(),
	}
	for i := range g.Grid {
		g.Grid[i] = make([]byte, cols)
		g.Winning[i] = make([]bool, cols)
	}
	placeBlocks(g.Grid, blocks)
	return g
}

func placeBlocks(grid [][]byte, n int) {
	h, w := len(grid), len(grid[0])
	tries := n * 10
	for n > 0 && tries > 0 {
		tries--
		r := mrand.Intn(h)
		c := mrand.Intn(w)
		if grid[r][c] == cellEmpty {
			grid[r][c] = cellBlk
			n--
		}
	}
}

// dropRow choisit la case d'arrivée dans la colonne col.
// Règle custom : rien ne bloque le chemin (ni R/Y ni X).
// On atterrit sur la case VIDE la plus éloignée dans le sens de la gravité.
// - Gravité normale (down)  : la case vide la PLUS BASSE
// - Gravité inversée (up)   : la case vide la PLUS HAUTE
// On ne peut pas atterrir sur une case 'X' (mais on peut "passer à travers").
func dropRow(grid [][]byte, col int, gravityUp bool) int {
	h := len(grid)
	if h == 0 || col < 0 || col >= len(grid[0]) {
		return -1
	}

	if !gravityUp {
		// vers le BAS : première case vide en partant du bas
		for r := h - 1; r >= 0; r-- {
			if grid[r][col] == cellEmpty {
				return r
			}
		}
		return -1 // aucune case vide
	}

	// vers le HAUT : première case vide en partant du haut
	for r := 0; r < h; r++ {
		if grid[r][col] == cellEmpty {
			return r
		}
	}
	return -1
}

func winningLine(grid [][]byte, r, c int, p byte) [][2]int {
	h, w := len(grid), len(grid[0])
	in := func(rr, cc int) bool { return rr >= 0 && rr < h && cc >= 0 && cc < w }
	dirs := [][2]int{{0, 1}, {1, 0}, {1, 1}, {1, -1}}
	for _, d := range dirs {
		line := [][2]int{{r, c}}
		rr, cc := r+d[0], c+d[1]
		for in(rr, cc) && grid[rr][cc] == p {
			line = append(line, [2]int{rr, cc})
			rr += d[0]
			cc += d[1]
		}
		rr, cc = r-d[0], c-d[1]
		for in(rr, cc) && grid[rr][cc] == p {
			line = append([][2]int{{rr, cc}}, line...)
			rr -= d[0]
			cc -= d[1]
		}
		if len(line) >= 4 {
			return line
		}
	}
	return nil
}

func isDraw(grid [][]byte) bool {
	for c := 0; c < len(grid[0]); c++ {
		if grid[0][c] == cellEmpty {
			return false
		}
	}
	return true
}

func (s *server) viewModel(g *Game) map[string]any {
	// indices
	colsIdx := make([]int, g.Cols)
	rowsIdx := make([]int, g.Rows)
	for i := 0; i < g.Cols; i++ {
		colsIdx[i] = i
	}
	for i := 0; i < g.Rows; i++ {
		rowsIdx[i] = i
	}

	// whose turn (only matters online)
	myTurn := true
	if g.Mode == "online" {
		if g.ThisIsRed {
			myTurn = (g.Current == cellR)
		} else {
			myTurn = (g.Current == cellY)
		}
	}

	// which columns are disabled?
	disabled := make([]bool, g.Cols)
	for c := 0; c < g.Cols; c++ {
		if !myTurn || g.GameOver {
			disabled[c] = true
			continue
		}
		disabled[c] = (dropRow(g.Grid, c, g.GravityUp) == -1)
	}

	return map[string]any{
		"Grid":       g.Grid,
		"PlayStart":  g.Turns == 0 && !g.GameOver,
		"Winning":    g.Winning,
		"Rows":       rowsIdx,
		"Cols":       colsIdx,
		"Disabled":   disabled,
		"CurrentStr": string(g.Current),
		"LastPlayed": string(g.LastPlayed), // requires: LastPlayed byte in Game
		"P1":         g.Player1,
		"P2":         g.Player2,
		"Scores":     g.Scores,
		"Message":    g.Message,
		"GravityUp":  g.GravityUp,
		"Turns":      g.Turns,
		"Difficulty": g.Difficulty,
		"GameOver":   g.GameOver,
		"IsOnline":   g.Mode == "online",
		"LobbyCode":  g.LobbyCode, // requires: LobbyCode string in Game
		"ThisIsRed":  g.ThisIsRed, // requires: ThisIsRed bool in Game
	}
}

func (s *server) render(w http.ResponseWriter, page string, data map[string]any) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if data == nil {
		data = map[string]any{}
	}
	data["Page"] = page // "start", "game", or "result"
	if err := s.tpl.ExecuteTemplate(w, "base", data); err != nil {
		http.Error(w, err.Error(), 500)
	}
}

func (s *server) gameForRequest(w http.ResponseWriter, r *http.Request, reset bool) *Game {
	s.mu.Lock()
	defer s.mu.Unlock()

	cookie, err := r.Cookie("pg_sid")
	if err != nil || cookie.Value == "" || reset {
		id := newID()
		g := newGame(6, 7, 3) // default (easy)
		s.sessions[id] = g
		http.SetCookie(w, &http.Cookie{
			Name:     "pg_sid",
			Value:    id,
			Path:     "/",
			HttpOnly: true,
			SameSite: http.SameSiteLaxMode,
			MaxAge:   60 * 60 * 24,
		})
		return g
	}
	if g, ok := s.sessions[cookie.Value]; ok {
		return g
	}
	g := newGame(6, 7, 3)
	s.sessions[cookie.Value] = g
	return g
}

func newID() string {
	b := make([]byte, 16)
	_, _ = crand.Read(b) // crypto-strong IDs for sessions
	return hex.EncodeToString(b)
}

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer-when-downgrade")
		w.Header().Set("X-Frame-Options", "DENY")
		// Allow inline JS (for tiny template scripts) and audio from self
		w.Header().Set("Content-Security-Policy",
			"default-src 'self'; script-src 'self' 'unsafe-inline'; style-src 'self'; img-src 'self' data:; media-src 'self';")

		next.ServeHTTP(w, r)
	})
}

/*** AI helpers ***/

func chooseAIMove(g *Game) int {
	bestCol := -1
	bestScore := -1_000_000
	for c := 0; c < g.Cols; c++ {
		r := dropRow(g.Grid, c, g.GravityUp)
		if r == -1 || g.Grid[r][c] != cellEmpty {
			continue
		}

		// try Y
		g.Grid[r][c] = cellY

		// winning now?
		if len(winningLine(g.Grid, r, c, cellY)) >= 4 {
			g.Grid[r][c] = cellEmpty
			return c
		}

		// block R immediate win?
		needBlock := false
		for cc := 0; cc < g.Cols && !needBlock; cc++ {
			rr := dropRow(g.Grid, cc, g.GravityUp)
			if rr == -1 || g.Grid[rr][cc] != cellEmpty {
				continue
			}
			g.Grid[rr][cc] = cellR
			if len(winningLine(g.Grid, rr, cc, cellR)) >= 4 {
				needBlock = true
			}
			g.Grid[rr][cc] = cellEmpty
		}

		score := evalBoard(g, cellY)
		if needBlock {
			score += 5000
		}
		center := g.Cols / 2
		score -= abs(c - center)

		g.Grid[r][c] = cellEmpty
		if score > bestScore {
			bestScore = score
			bestCol = c
		}
	}
	return bestCol
}

func evalBoard(g *Game, me byte) int {
	op := cellR
	if me == cellR {
		op = cellY
	}

	countK := func(p byte, k int) int {
		h, w := len(g.Grid), len(g.Grid[0])
		dirs := [][2]int{{0, 1}, {1, 0}, {1, 1}, {1, -1}}
		total := 0
		in := func(r, c int) bool { return r >= 0 && r < h && c >= 0 && c < w }
		for r := 0; r < h; r++ {
			for c := 0; c < w; c++ {
				for _, d := range dirs {
					cnt := 0
					rr, cc := r, c
					clear := true
					for i := 0; i < k; i++ {
						if !in(rr, cc) || g.Grid[rr][cc] == cellBlk {
							clear = false
							break
						}
						if g.Grid[rr][cc] == p {
							cnt++
						}
						rr += d[0]
						cc += d[1]
					}
					if clear && cnt == k {
						total++
					}
				}
			}
		}
		return total
	}

	return 50*countK(me, 3) + 10*countK(me, 2) -
		50*countK(op, 3) - 10*countK(op, 2)
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

/*** Online handlers (MVP, in-memory) ***/

func (s *server) newLobbyCode() string {
	const letters = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"
	b := make([]byte, 4)
	for i := range b {
		b[i] = letters[mrand.Intn(len(letters))]
	}
	return string(b)
}

func (s *server) handleOnlineCreate(w http.ResponseWriter, r *http.Request) {
	rows, _ := strconv.Atoi(r.URL.Query().Get("rows"))
	cols, _ := strconv.Atoi(r.URL.Query().Get("cols"))
	blocks, _ := strconv.Atoi(r.URL.Query().Get("blocks"))
	if rows <= 0 {
		rows = 6
	}
	if cols <= 0 {
		cols = 7
	}

	p1 := r.URL.Query().Get("p1")
	if p1 == "" {
		p1 = "Rouge"
	}
	p2 := r.URL.Query().Get("p2")
	if p2 == "" {
		p2 = "Jaune"
	}
	diff := r.URL.Query().Get("diff")
	if diff == "" {
		diff = "easy"
	}

	// NEW: allow custom code if provided
	code := strings.ToUpper(strings.TrimSpace(r.URL.Query().Get("code")))
	if len(code) < 4 {
		code = s.newLobbyCode()
	}

	// avoid collisions
	s.mu.Lock()
	if _, exists := s.lobbies[code]; exists {
		s.mu.Unlock()
		// simple UX: send back to start if code taken (you can render a page instead)
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	g := newGame(rows, cols, blocks)
	g.Player1, g.Player2 = p1, p2
	g.Difficulty = diff
	g.Mode = "online"
	g.LobbyCode = code
	g.ThisIsRed = true

	lb := &lobby{Game: g, UpdatedAt: time.Now(), HasRed: true}
	s.lobbies[code] = lb
	s.mu.Unlock()

	http.Redirect(w, r, "/online/wait?code="+code+"&side=R", http.StatusSeeOther)
}

func (s *server) handleOnlineJoin(w http.ResponseWriter, r *http.Request) {
	code := strings.ToUpper(strings.TrimSpace(r.URL.Query().Get("code")))
	if code == "" {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	s.mu.Lock()
	lb, ok := s.lobbies[code]
	if ok && !lb.HasYellow {
		lb.HasYellow = true
		lb.UpdatedAt = time.Now()
	}
	s.mu.Unlock()

	if !ok {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	http.Redirect(w, r, "/online/wait?code="+code+"&side=Y", http.StatusSeeOther)
}

func (s *server) handleOnlineWait(w http.ResponseWriter, r *http.Request) {
	code := strings.ToUpper(strings.TrimSpace(r.URL.Query().Get("code")))
	side := strings.ToUpper(strings.TrimSpace(r.URL.Query().Get("side")))
	if code == "" || (side != "R" && side != "Y") {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	s.mu.Lock()
	lb, ok := s.lobbies[code]
	s.mu.Unlock()
	if !ok {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	gcopy := *lb.Game
	gcopy.LobbyCode = code
	gcopy.Mode = "online"
	gcopy.ThisIsRed = (side == "R")

	data := s.viewModel(&gcopy)
	data["LobbyCode"] = code
	data["IsOnline"] = true
	s.render(w, "game", data)
}

func (s *server) handleOnlineState(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	// prevent any caching of the JSON response
	w.Header().Set("Cache-Control", "no-store, no-cache, must-revalidate")
	w.Header().Set("Pragma", "no-cache")

	code := strings.ToUpper(strings.TrimSpace(r.URL.Query().Get("code")))
	if code == "" {
		w.WriteHeader(400)
		_, _ = w.Write([]byte(`{"err":"missing code"}`))
		return
	}

	s.mu.Lock()
	lb, ok := s.lobbies[code]
	s.mu.Unlock()
	if !ok {
		w.WriteHeader(404)
		_, _ = w.Write([]byte(`{"err":"not found"}`))
		return
	}

	_, _ = w.Write([]byte(fmt.Sprintf(
		`{"ok":true,"gameOver":%t,"current":"%s","gravityUp":%t,"turns":%d}`,
		lb.Game.GameOver, string(lb.Game.Current), lb.Game.GravityUp, lb.Game.Turns,
	)))
}

func (s *server) handleOnlinePlay(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	code := strings.ToUpper(strings.TrimSpace(r.FormValue("code")))
	side := strings.ToUpper(strings.TrimSpace(r.FormValue("side")))
	colStr := r.FormValue("col")
	c, _ := strconv.Atoi(colStr)

	s.mu.Lock()
	lb, ok := s.lobbies[code]
	if !ok {
		s.mu.Unlock()
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	g := lb.Game

	expect := cellR
	if side == "Y" {
		expect = cellY
	}
	if g.Current != expect || g.GameOver {
		s.mu.Unlock()
		http.Redirect(w, r, "/online/wait?code="+code+"&side="+side, http.StatusSeeOther)
		return
	}

	row := dropRow(g.Grid, c, g.GravityUp)
	if row == -1 || g.Grid[row][c] != cellEmpty {
		s.mu.Unlock()
		http.Redirect(w, r, "/online/wait?code="+code+"&side="+side, http.StatusSeeOther)
		return
	}
	g.Grid[row][c] = g.Current
	g.Turns++

	if s.checkResult(g, row, c, g.Current) {
		lb.UpdatedAt = time.Now()
		s.mu.Unlock()

		// ⬇️ Copy the finished lobby game into this user's session,
		// so /result renders the correct names/scores/LastPlayed.
		gs := s.gameForRequest(w, r, true) // reset or create the session game
		*gs = *g                           // shallow copy is fine (we don't reuse it)

		http.Redirect(w, r, "/result", http.StatusSeeOther)
		return
	}

	if g.Current == cellR {
		g.Current = cellY
	} else {
		g.Current = cellR
	}
	if g.Turns%5 == 0 {
		g.GravityUp = !g.GravityUp
		g.Message = ""
	}

	lb.UpdatedAt = time.Now()
	s.mu.Unlock()

	http.Redirect(w, r, "/online/wait?code="+code+"&side="+side, http.StatusSeeOther)
}

// POST /chat/post  (form: code, side, name, text)
func (s *server) handleChatPost(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method", http.StatusMethodNotAllowed)
		return
	}

	code := strings.ToUpper(strings.TrimSpace(r.FormValue("code")))
	side := strings.ToUpper(strings.TrimSpace(r.FormValue("side"))) // "R" ou "Y"
	name := strings.TrimSpace(r.FormValue("name"))
	text := strings.TrimSpace(r.FormValue("text"))

	if code == "" || (side != "R" && side != "Y") || text == "" {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	if len(text) > 240 { // petite limite
		text = text[:240]
	}
	if name == "" {
		name = "Joueur"
	}

	s.mu.Lock()
	lb, ok := s.lobbies[code]
	if !ok {
		s.mu.Unlock()
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	lb.NextChatID++
	msg := ChatMessage{
		ID:   lb.NextChatID,
		When: time.Now(),
		Side: side,
		Name: name,
		Text: text,
	}
	lb.Chat = append(lb.Chat, msg)
	// cap à ~200 messages
	if len(lb.Chat) > 200 {
		lb.Chat = lb.Chat[len(lb.Chat)-200:]
	}
	lb.UpdatedAt = time.Now()
	s.mu.Unlock()

	w.WriteHeader(http.StatusNoContent)
}

// GET /chat/feed?code=ABCD&since=123
func (s *server) handleChatFeed(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store, no-cache, must-revalidate")
	w.Header().Set("Pragma", "no-cache")

	code := strings.ToUpper(strings.TrimSpace(r.URL.Query().Get("code")))
	sinceStr := strings.TrimSpace(r.URL.Query().Get("since"))
	var since int64
	if sinceStr != "" {
		if v, err := strconv.ParseInt(sinceStr, 10, 64); err == nil {
			since = v
		}
	}
	if code == "" {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"err":"missing code"}`))
		return
	}

	s.mu.Lock()
	lb, ok := s.lobbies[code]
	var out []ChatMessage
	if ok {
		for _, m := range lb.Chat {
			if m.ID > since {
				out = append(out, m)
			}
		}
	}
	s.mu.Unlock()
	if !ok {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"err":"not found"}`))
		return
	}

	// petite réponse JSON
	builder := strings.Builder{}
	builder.WriteString(`{"ok":true,"items":[`)
	for i, m := range out {
		if i > 0 {
			builder.WriteByte(',')
		}
		// échappes basiques
		txt := strings.ReplaceAll(m.Text, `"`, `\"`)
		nm := strings.ReplaceAll(m.Name, `"`, `\"`)
		builder.WriteString(fmt.Sprintf(`{"id":%d,"side":"%s","name":"%s","text":"%s"}`, m.ID, m.Side, nm, txt))
	}
	builder.WriteString(`]}`)
	_, _ = w.Write([]byte(builder.String()))
}
