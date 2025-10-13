package main

import (
	"crypto/rand"
	_ "embed"
	"encoding/hex"
	"html/template"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

//go:embed templates/base.gohtml
var baseTpl string

//go:embed templates/index.gohtml
var indexTpl string

//go:embed static/style.css
var cssBytes []byte

const (
	rows = 6
	cols = 7
)

type Cell = byte // 0 empty, 'R', 'Y'

type Game struct {
	Grid       [rows][cols]Cell
	Current    Cell   // 'R' or 'Y'
	Mode       string // "pvp" or "ai"
	Scores     struct{ R, Y int }
	Message    string
	Winning    [rows][cols]bool
	GameOver   bool
	LastPlayed time.Time
}

type server struct {
	tpl      *template.Template
	sessions map[string]*Game
	mu       sync.Mutex
}

func main() {
	s := &server{
		tpl:      template.Must(template.New("base").Parse(baseTpl + indexTpl)),
		sessions: make(map[string]*Game),
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/", s.handleIndex)
	mux.HandleFunc("/play", s.handlePlay)
	mux.HandleFunc("/reset", s.handleReset)
	mux.HandleFunc("/replay", s.handleReplay)
	mux.HandleFunc("/mode", s.handleMode)
	mux.HandleFunc("/static/style.css", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/css; charset=utf-8")
		w.Write(cssBytes)
	})
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"ok":true}`))
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

	log.Printf("Puissance 4 (no-JS) listening on :%s\n", port)
	log.Fatal(srv.ListenAndServe())
}

func (s *server) handleIndex(w http.ResponseWriter, r *http.Request) {
	g := s.gameForRequest(w, r)
	data := s.viewModel(g)
	s.render(w, data)
}

func (s *server) handleMode(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	g := s.gameForRequest(w, r)
	mode := strings.ToLower(r.FormValue("mode"))
	if mode != "ai" {
		mode = "pvp"
	}
	g.Mode = mode
	g.Message = ""
	g.GameOver = false
	g.Winning = [rows][cols]bool{}
	s.redirect(w, r, "/")
}

func (s *server) handleReset(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	g := s.gameForRequest(w, r)
	*g = *newGame() // reset complet (scores remis à 0)
	s.redirect(w, r, "/")
}

func (s *server) handleReplay(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	g := s.gameForRequest(w, r)
	mode := g.Mode
	scoreR, scoreY := g.Scores.R, g.Scores.Y
	*g = *newGame()
	g.Mode = mode
	g.Scores.R, g.Scores.Y = scoreR, scoreY
	s.redirect(w, r, "/")
}

func (s *server) handlePlay(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	g := s.gameForRequest(w, r)
	if g.GameOver {
		s.redirect(w, r, "/")
		return
	}

	colStr := r.FormValue("col")
	c, err := strconv.Atoi(colStr)
	if err != nil || c < 0 || c >= cols {
		s.redirect(w, r, "/")
		return
	}

	// Joueur courant joue
	row := dropRow(&g.Grid, c)
	if row == -1 {
		// colonne pleine -> rien
		s.redirect(w, r, "/")
		return
	}
	g.Grid[row][c] = g.Current

	// Check victoire / égalité
	if s.checkResult(g, row, c, g.Current) {
		// gameOver et scores mis à jour dans checkResult
		s.redirect(w, r, "/")
		return
	}

	// Switch tour
	if g.Current == 'R' {
		g.Current = 'Y'
	} else {
		g.Current = 'R'
	}

	// Si mode IA et c'est au bot (Jaune), il joue tout de suite
	if g.Mode == "ai" && g.Current == 'Y' && !g.GameOver {
		s.aiMove(g)
	}
	s.redirect(w, r, "/")
}

func (s *server) aiMove(g *Game) {
	// 1) coup gagnant pour 'Y'
	if col := findWinningMove(g.Grid, 'Y'); col != -1 {
		row := dropRow(&g.Grid, col)
		g.Grid[row][col] = 'Y'
		if s.checkResult(g, row, col, 'Y') {
			return
		}
		g.Current = 'R'
		return
	}
	// 2) bloquer 'R'
	if col := findWinningMove(g.Grid, 'R'); col != -1 {
		row := dropRow(&g.Grid, col)
		g.Grid[row][col] = 'Y'
		if s.checkResult(g, row, col, 'Y') {
			return
		}
		g.Current = 'R'
		return
	}
	// 3) centre
	if r := dropRow(&g.Grid, 3); r != -1 {
		g.Grid[r][3] = 'Y'
		if s.checkResult(g, r, 3, 'Y') {
			return
		}
		g.Current = 'R'
		return
	}
	// 4) proche du centre
	order := []int{2, 4, 1, 5, 0, 6}
	for _, c := range order {
		if r := dropRow(&g.Grid, c); r != -1 {
			g.Grid[r][c] = 'Y'
			if s.checkResult(g, r, c, 'Y') {
				return
			}
			g.Current = 'R'
			return
		}
	}
}

func (s *server) checkResult(g *Game, r, c int, p Cell) bool {
	if line := winningLine(g.Grid, r, c, p); len(line) >= 4 {
		for _, rc := range line {
			g.Winning[rc[0]][rc[1]] = true
		}
		g.GameOver = true
		if p == 'R' {
			g.Scores.R++
		} else {
			g.Scores.Y++
		}
		if p == 'R' {
			g.Message = "🎉 Victoire de Rouge !"
		} else {
			g.Message = "🎉 Victoire de Jaune !"
		}
		return true
	}
	if isDraw(g.Grid) {
		g.GameOver = true
		g.Message = "🤝 Égalité !"
		return true
	}
	return false
}

func dropRow(grid *[rows][cols]Cell, c int) int {
	for r := rows - 1; r >= 0; r-- {
		if grid[r][c] == 0 {
			return r
		}
	}
	return -1
}

func winningLine(grid [rows][cols]Cell, r, c int, p Cell) [][2]int {
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
			return line[:4]
		}
	}
	return nil
}

func in(r, c int) bool { return r >= 0 && r < rows && c >= 0 && c < cols }

func isDraw(grid [rows][cols]Cell) bool {
	for c := 0; c < cols; c++ {
		if grid[0][c] == 0 {
			return false
		}
	}
	return true
}

func findWinningMove(grid [rows][cols]Cell, player Cell) int {
	for c := 0; c < cols; c++ {
		r := -1
		for rr := rows - 1; rr >= 0; rr-- {
			if grid[rr][c] == 0 {
				r = rr
				break
			}
		}
		if r == -1 {
			continue
		}
		grid[r][c] = player
		if line := winningLine(grid, r, c, player); len(line) >= 4 {
			return c
		}
		grid[r][c] = 0
	}
	return -1
}

func (s *server) viewModel(g *Game) map[string]any {
	colsIdx := make([]int, cols)
	rowsIdx := make([]int, rows)
	for i := 0; i < cols; i++ {
		colsIdx[i] = i
	}
	for i := 0; i < rows; i++ {
		rowsIdx[i] = i
	}
	disabled := make([]bool, cols)
	for c := 0; c < cols; c++ {
		disabled[c] = g.GameOver || (g.Grid[0][c] != 0)
	}
	return map[string]any{
		"Grid":     g.Grid,
		"Current":  string(g.Current),
		"Mode":     g.Mode,
		"Scores":   g.Scores,
		"Message":  g.Message,
		"Winning":  g.Winning,
		"Cols":     colsIdx,
		"Rows":     rowsIdx,
		"Disabled": disabled,
	}
}

func (s *server) render(w http.ResponseWriter, data map[string]any) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := s.tpl.ExecuteTemplate(w, "base", data); err != nil {
		http.Error(w, err.Error(), 500)
	}
}

func (s *server) redirect(w http.ResponseWriter, r *http.Request, to string) {
	http.Redirect(w, r, to, http.StatusSeeOther)
}

func (s *server) gameForRequest(w http.ResponseWriter, r *http.Request) *Game {
	s.mu.Lock()
	defer s.mu.Unlock()

	cookie, err := r.Cookie("pg_sid")
	if err != nil || cookie.Value == "" {
		id := newID()
		g := newGame()
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
		g.LastPlayed = time.Now()
		return g
	}
	g := newGame()
	s.sessions[cookie.Value] = g
	return g
}

func newGame() *Game {
	g := &Game{
		Current:    'R',
		Mode:       "pvp",
		LastPlayed: time.Now(),
	}
	return g
}

func newID() string {
	b := make([]byte, 16)
	rand.Read(b)
	return hex.EncodeToString(b)
}

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer-when-downgrade")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; style-src 'self'; img-src 'self' data:;")
		next.ServeHTTP(w, r)
	})
}
