# Puissance 4 — Projet Ynov (Elias & Alan)

Un **Puissance 4 moderne** codé **100% en Go**, avec interface web en mode **glassmorphism**, sons, IA et mode en ligne.

Projet réalisé dans le cadre de nos études à **Ynov** par **Elias** et **Alan**.

---

## 🕹️ Fonctionnalités

- 🎮 **3 modes de jeu**
  - **Local** : 2 joueurs sur le même PC
  - **IA** : jouer contre une IA (avec un minimum de "cerveau" 😄)
  - **En ligne** : 2 joueurs sur des PC différents via un système de salle (code de lobby)

- 📊 **Niveaux de difficulté**
  - *Easy* — 6×7 avec 3 blocs
  - *Normal* — 6×8 avec 5 blocs
  - *Hard* — 6×9 avec 7 blocs
  - Des blocs immobiles (`X`) changent les stratégies possibles

- 🧲 **Gravité dynamique**
  - La gravité change toutes les 5 actions :
    - Gravité normale : les pions tombent vers le bas
    - Gravité inversée : les pions montent vers le haut
  - Indicateur visuel de l’état de gravité

- 🌐 **Mode en ligne**
  - Création de **salles** avec un code
  - Rejoint via un code existant
  - Affichage du code de salle
  - Système de **revanche** avec votes (1/2, 2/2 prêts)

- 💬 **Mini-chat in-game (en ligne)**
  - Chat temps réel entre les deux joueurs de la salle
  - Messages colorés selon le joueur (Rouge / Jaune)
  - Polling léger côté client (fetch JSON)

- 🔊 **Ambiance sonore**
  - Musique de fond (toggle dans l’interface)
  - Sons :
    - clic
    - drop des pions
    - victoire
    - début de partie

- 🎨 **UI moderne**
  - Design glassmorphism
  - Animations de pions “glossy”
  - Indicateurs de score
  - Affichage du gagnant : `Victoire de <nom du joueur> !`

---

## 🧰 Stack technique

- **Langage** : Go (Golang)
- **Version minimale** : Go `1.21`
- **Standard lib uniquement** :
  - `net/http` pour le serveur
  - `html/template` pour les pages
  - pas de framework externe
- Front :
  - HTML / CSS pur
  - Un peu de JavaScript pour :
    - Musique
    - Sons
    - Mode online (polling)
    - Chat

---

## 🚀 Lancer le projet en local

### 1. Prérequis

- Go installé (version **1.21+**)
  - Vérifier avec :
    ```bash
    go version
    ```

### 2. Cloner le repo

```bash
git clone https://github.com/<ton-user>/<ton-repo>.git
cd <ton-repo>
