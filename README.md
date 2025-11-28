# 🌟 Puissance 4 — Projet Ynov (Elias & Alan)

Un **Puissance 4 nouvelle génération**, développé **100% en Go**, avec :

- 🎨 Interface web en **glassmorphism**
- 🔊 Effets sonores complets
- 🤖 IA intégrée
- 🌐 Mode en ligne avec chat
- 🧲 Gravité dynamique

Projet créé dans le cadre de Ynov par **Elias** et **Alan**.

---

## 🕹️ Fonctionnalités

### 🎮 Modes de jeu
- **Local** — 2 joueurs sur le même PC  
- **IA** — IA intégrée avec logique et stratégie  
- **En ligne** — Jouer à 2 sur des PC différents via un code de lobby

### 📊 Difficultés
| Difficulté | Grille | Blocs |
|------------|--------|--------|
| Easy       | 6×7    | 3      |
| Normal     | 6×8    | 5      |
| Hard       | 6×9    | 7      |

Des blocs immobiles (`X`) changent totalement la stratégie du jeu.

### 🧲 Gravité dynamique
La gravité change **toutes les 5 actions** :
- Gravité normale → les pions tombent  
- Gravité inversée → les pions montent  

Avec un indicateur visuel dynamique.

### 🌐 Mode en ligne
- Création de salle (code automatique ou personnalisé)
- Rejoindre avec un code
- Synchronisation continue (polling JSON)
- Page de résultat partagée
- Fonction **Revanche** (votes 0/2 → 2/2)

### 💬 Mini-chat intégré
- Chat en temps réel
- Messages colorés selon le joueur (Rouge / Jaune)
- Scroll automatique  
- Requêtes légères

### 🔊 Ambiance sonore
- Musique de fond (toggle + sauvegarde)
- Sons :
  - clic
  - dépôt
  - gravité inversée
  - victoire
  - début de partie

### 🎨 UI moderne
- Glassmorphism
- Grille responsive
- Animations glossy
- Effet visuel dynamique sur la page de démarrage

---

## 🧰 Stack technique

| Domaine | Choix |
|--------|-------|
| Langage | Go (Golang) |
| Minimum | Go **1.21+** |
| Backend | `net/http` + `html/template` |
| Frontend | HTML, CSS, JS vanilla |
| Temps réel | Polling JSON |
| Sessions | Cookies + mémoire |

Aucune dépendance externe → fonctionne partout.

---

## 🚀 Installation & Lancement

### 1) Installer Go
Vérifie ta version :

```bash
go version
Il faut Go 1.21 ou supérieur.

2) Cloner le projet
git clone https://github.com/el211/power4-web
cd power4-web

3) Lancer le serveur

Méthode simple :

go run .


Ou compiler puis lancer :

go build -o power4
./power4


Tu devrais avoir :

Power4 BONUS listening on :8080

4) Jouer 🎮

Ouvre ton navigateur à l’adresse :

👉 http://localhost:8080/

📁 Structure du projet
power4-web/
 ├─ static/
 │   ├─ style.css
 │   ├─ js/
 │   ├─ sounds/
 │   └─ images/
 ├─ templates/
 │   ├─ base.html
 │   ├─ start.html
 │   ├─ game.html
 │   └─ result.html
 ├─ main.go
 └─ README.md