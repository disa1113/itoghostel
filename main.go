package main

import (
	"database/sql"
	"fmt"
	"html/template"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/sessions"
	_ "github.com/lib/pq"
	"golang.org/x/crypto/bcrypt"
)

type User struct {
	ID        int
	Username  string
	Email     string
	Password  string
	Role      string
	CreatedAt string
}

type Room struct {
	ID          int
	Name        string
	Description string
	Price       float64
	Capacity    int
}

type Booking struct {
	ID        int
	UserID    int
	RoomID    int
	RoomName  string
	UserName  string
	CheckIn   string
	CheckOut  string
	Guests    int
	Status    string
	Price     float64
	CreatedAt string
}

var (
	db    *sql.DB
	mu    sync.RWMutex
	tmpl  *template.Template
	store = sessions.NewCookieStore([]byte("hostel-secret-key-2024"))
)

// Конфигурация для Render (через переменные окружения)
var (
	dbHost     = os.Getenv("DB_HOST")
	dbPort     = os.Getenv("DB_PORT")
	dbUser     = os.Getenv("DB_USER")
	dbPassword = os.Getenv("DB_PASSWORD")
	dbName     = os.Getenv("DB_NAME")
)

func init() {
	// Настройки сессии для продакшена
	store.Options = &sessions.Options{
		Path:     "/",
		MaxAge:   86400 * 7, // 7 дней
		HttpOnly: true,
		Secure:   false, // для HTTP на Render; если будет HTTPS, нужно сменить на true
		SameSite: http.SameSiteLaxMode,
	}

	// Значения по умолчанию для локальной разработки
	if dbHost == "" {
		dbHost = "localhost"
	}
	if dbPort == "" {
		dbPort = "5432"
	}
	if dbUser == "" {
		dbUser = "postgres"
	}
	if dbName == "" {
		dbName = "hostel_db"
	}

	initDB()

	funcMap := template.FuncMap{
		"now": func() string {
			return time.Now().Format("2006-01-02")
		},
		"seq": func(start, end int) []int {
			var result []int
			for i := start; i <= end; i++ {
				result = append(result, i)
			}
			return result
		},
		"pluralize": func(n int, one, few, many string) string {
			if n%10 == 1 && n%100 != 11 {
				return one
			} else if n%10 >= 2 && n%10 <= 4 && (n%100 < 10 || n%100 >= 20) {
				return few
			}
			return many
		},
	}

	var err error
	tmpl, err = template.New("").Funcs(funcMap).ParseGlob("templates/*.html")
	if err != nil {
		log.Fatalf("Ошибка загрузки шаблонов: %v", err)
	}
	log.Println("Шаблоны загружены")
}

func initDB() {
	dsn := fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=disable",
		dbHost, dbPort, dbUser, dbPassword, dbName)

	var err error
	db, err = sql.Open("postgres", dsn)
	if err != nil {
		log.Fatalf("Ошибка подключения к PostgreSQL: %v", err)
	}

	if err = db.Ping(); err != nil {
		log.Printf("Не удалось подключиться, создаю базу...")
		createDatabase()
	} else {
		log.Println("PostgreSQL подключен")
	}

	// Выполняем SQL файл если он существует
	if _, err := os.Stat("database.sql"); err == nil {
		log.Println("Выполняю database.sql...")
		executeSQLFile("database.sql")
	} else {
		createTables()
		insertTestData()
	}
}

func createDatabase() {
	// Подключаемся без указания БД
	dsn := fmt.Sprintf("host=%s port=%s user=%s password=%s sslmode=disable",
		dbHost, dbPort, dbUser, dbPassword)

	tempDB, err := sql.Open("postgres", dsn)
	if err != nil {
		log.Fatal(err)
	}
	defer tempDB.Close()

	// Создаём базу данных
	_, err = tempDB.Exec(fmt.Sprintf("CREATE DATABASE %s", dbName))
	if err != nil {
		log.Printf("База данных %s уже существует или ошибка: %v", dbName, err)
	}

	// Переподключаемся к созданной БД
	dsnFull := fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=disable",
		dbHost, dbPort, dbUser, dbPassword, dbName)

	db, err = sql.Open("postgres", dsnFull)
	if err != nil {
		log.Fatal(err)
	}
	log.Println("База данных создана/подключена")
}

func executeSQLFile(filename string) error {
	content, err := os.ReadFile(filename)
	if err != nil {
		log.Printf("Не удалось прочитать файл %s: %v", filename, err)
		return err
	}

	queries := strings.Split(string(content), ";")
	for _, query := range queries {
		query = strings.TrimSpace(query)
		if query == "" || strings.HasPrefix(query, "--") {
			continue
		}
		_, err := db.Exec(query)
		if err != nil {
			log.Printf("SQL ошибка: %v", err)
		}
	}
	log.Println("SQL файл выполнен")
	return nil
}

func createTables() {
	queries := []string{
		`CREATE TABLE IF NOT EXISTS users (
			id SERIAL PRIMARY KEY,
			username VARCHAR(50) UNIQUE NOT NULL,
			email VARCHAR(100) UNIQUE NOT NULL,
			password VARCHAR(255) NOT NULL,
			role VARCHAR(20) DEFAULT 'user',
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS rooms (
			id SERIAL PRIMARY KEY,
			name VARCHAR(100) NOT NULL,
			description TEXT,
			price DECIMAL(10,2) NOT NULL,
			capacity INT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS bookings (
			id SERIAL PRIMARY KEY,
			user_id INT NOT NULL,
			room_id INT NOT NULL,
			check_in DATE NOT NULL,
			check_out DATE NOT NULL,
			guests INT NOT NULL,
			status VARCHAR(20) DEFAULT 'pending',
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
			FOREIGN KEY (room_id) REFERENCES rooms(id) ON DELETE CASCADE
		)`,
	}

	for _, q := range queries {
		_, err := db.Exec(q)
		if err != nil {
			log.Printf("Ошибка таблицы: %v", err)
		}
	}
	log.Println("Таблицы созданы")
}

func insertTestData() {
	var count int
	db.QueryRow("SELECT COUNT(*) FROM users WHERE role = 'manager'").Scan(&count)
	if count == 0 {
		hash, _ := bcrypt.GenerateFromPassword([]byte("manager123"), bcrypt.DefaultCost)
		db.Exec("INSERT INTO users (username, email, password, role) VALUES ($1, $2, $3, $4)",
			"manager", "manager@hostel.com", string(hash), "manager")

		hash2, _ := bcrypt.GenerateFromPassword([]byte("admin123"), bcrypt.DefaultCost)
		db.Exec("INSERT INTO users (username, email, password, role) VALUES ($1, $2, $3, $4)",
			"newmanager", "newmanager@hostel.com", string(hash2), "manager")

		hash3, _ := bcrypt.GenerateFromPassword([]byte("user123"), bcrypt.DefaultCost)
		db.Exec("INSERT INTO users (username, email, password) VALUES ($1, $2, $3)",
			"user", "user@hostel.com", string(hash3))
	}

	db.QueryRow("SELECT COUNT(*) FROM rooms").Scan(&count)
	if count == 0 {
		rooms := [][]interface{}{
			{"Эконом 6-местный", "Бюджетный номер на 6 человек", 800, 6},
			{"Стандарт 4-местный", "Комфортабельный номер на 4 человека", 1200, 4},
			{"Комфорт 2-местный", "Уютный двухместный номер", 1800, 2},
			{"Делюкс 2-местный", "Просторный двухместный номер", 2500, 2},
			{"Премиум 1-местный", "Люкс для одного гостя", 3200, 1},
			{"Семейный люкс", "Просторный семейный номер", 4500, 4},
		}
		for _, r := range rooms {
			db.Exec("INSERT INTO rooms (name, description, price, capacity) VALUES ($1, $2, $3, $4)",
				r[0], r[1], r[2], r[3])
		}
	}
	log.Println("Тестовые данные добавлены")
}

func hashPassword(p string) (string, error) {
	b, err := bcrypt.GenerateFromPassword([]byte(p), bcrypt.DefaultCost)
	return string(b), err
}

func checkPasswordHash(p, h string) bool {
	return bcrypt.CompareHashAndPassword([]byte(h), []byte(p)) == nil
}

func authMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		s, _ := store.Get(r, "session")
		if auth, ok := s.Values["authenticated"].(bool); !ok || !auth {
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}
		next(w, r)
	}
}

func managerMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		s, _ := store.Get(r, "session")
		if role, ok := s.Values["role"].(string); !ok || role != "manager" {
			http.Error(w, "Доступ запрещен", http.StatusForbidden)
			return
		}
		next(w, r)
	}
}

func openBrowser(url string) {
	var err error
	switch runtime.GOOS {
	case "windows":
		err = exec.Command("rundll32", "url.dll,FileProtocolHandler", url).Start()
	case "darwin":
		err = exec.Command("open", url).Start()
	case "linux":
		err = exec.Command("xdg-open", url).Start()
	}
	if err != nil {
		log.Printf("Не открылся браузер: %v", err)
	}
}

func main() {
	fs := http.FileServer(http.Dir("static"))
	http.Handle("/static/", http.StripPrefix("/static/", fs))

	http.HandleFunc("/", homeHandler)
	http.HandleFunc("/login", loginHandler)
	http.HandleFunc("/register", registerHandler)
	http.HandleFunc("/logout", logoutHandler)
	http.HandleFunc("/dashboard", authMiddleware(userDashboardHandler))
	http.HandleFunc("/manager", authMiddleware(managerMiddleware(managerDashboardHandler)))
	http.HandleFunc("/rooms", roomsHandler)
	http.HandleFunc("/book-room", bookFormHandler)
	http.HandleFunc("/book", bookRoomHandler)
	http.HandleFunc("/cancel-booking", authMiddleware(cancelBookingHandler))
	http.HandleFunc("/update-booking", authMiddleware(managerMiddleware(updateBookingHandler)))

	port := ":8080"
	url := "http://localhost" + port

	fmt.Println("=========================================")
	fmt.Println("         Хостел Уют")
	fmt.Println("=========================================")
	fmt.Printf("Ссылка: %s\n", url)
	fmt.Println("=========================================")

	ln, err := net.Listen("tcp", port)
	if err != nil {
		log.Fatal(err)
	}
	go openBrowser(url)
	log.Fatal(http.Serve(ln, nil))
}

func homeHandler(w http.ResponseWriter, r *http.Request) {
	session, _ := store.Get(r, "session")
	isAuth, _ := session.Values["authenticated"].(bool)
	role, _ := session.Values["role"].(string)

	rows, err := db.Query("SELECT id, name, description, price, capacity FROM rooms LIMIT 3")
	if err != nil {
		http.Error(w, "Ошибка загрузки номеров", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var rooms []Room
	for rows.Next() {
		var room Room
		err := rows.Scan(&room.ID, &room.Name, &room.Description, &room.Price, &room.Capacity)
		if err != nil {
			continue
		}
		rooms = append(rooms, room)
	}

	data := struct {
		Rooms           []Room
		IsAuthenticated bool
		Role            string
	}{
		Rooms:           rooms,
		IsAuthenticated: isAuth,
		Role:            role,
	}

	tmpl.ExecuteTemplate(w, "index.html", data)
}

func loginHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method == "POST" {
		username := r.FormValue("username")
		password := r.FormValue("password")

		var user User
		var hashed string
		err := db.QueryRow("SELECT id, username, email, role, password FROM users WHERE username=$1", username).
			Scan(&user.ID, &user.Username, &user.Email, &user.Role, &hashed)

		if err != nil || !checkPasswordHash(password, hashed) {
			tmpl.ExecuteTemplate(w, "login.html", map[string]interface{}{"Error": "Неверные данные"})
			return
		}

		s, _ := store.Get(r, "session")
		s.Values["authenticated"] = true
		s.Values["user_id"] = user.ID
		s.Values["username"] = user.Username
		s.Values["role"] = user.Role
		s.Save(r, w)

		if user.Role == "manager" {
			http.Redirect(w, r, "/manager", http.StatusSeeOther)
		} else {
			http.Redirect(w, r, "/dashboard", http.StatusSeeOther)
		}
		return
	}
	tmpl.ExecuteTemplate(w, "login.html", nil)
}

func registerHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method == "POST" {
		username := r.FormValue("username")
		email := r.FormValue("email")
		password := r.FormValue("password")
		confirm := r.FormValue("confirm_password")

		if password != confirm {
			tmpl.ExecuteTemplate(w, "register.html", map[string]interface{}{"Error": "Пароли не совпадают"})
			return
		}

		var exists bool
		db.QueryRow("SELECT EXISTS(SELECT 1 FROM users WHERE username=$1 OR email=$2)", username, email).Scan(&exists)
		if exists {
			tmpl.ExecuteTemplate(w, "register.html", map[string]interface{}{"Error": "Пользователь уже существует"})
			return
		}

		hash, _ := hashPassword(password)
		res, err := db.Exec("INSERT INTO users (username, email, password) VALUES ($1, $2, $3)", username, email, hash)
		if err != nil {
			tmpl.ExecuteTemplate(w, "register.html", map[string]interface{}{"Error": "Ошибка регистрации"})
			return
		}

		id, _ := res.LastInsertId()
		s, _ := store.Get(r, "session")
		s.Values["authenticated"] = true
		s.Values["user_id"] = int(id)
		s.Values["username"] = username
		s.Values["role"] = "user"
		s.Save(r, w)

		http.Redirect(w, r, "/dashboard", http.StatusSeeOther)
		return
	}
	tmpl.ExecuteTemplate(w, "register.html", nil)
}

func logoutHandler(w http.ResponseWriter, r *http.Request) {
	s, _ := store.Get(r, "session")
	s.Values["authenticated"] = false
	s.Save(r, w)
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func userDashboardHandler(w http.ResponseWriter, r *http.Request) {
	s, _ := store.Get(r, "session")
	userID := s.Values["user_id"].(int)
	username := s.Values["username"].(string)

	rows, err := db.Query(`
		SELECT b.id, b.check_in, b.check_out, b.guests, b.status, b.created_at, r.name, r.price
		FROM bookings b JOIN rooms r ON b.room_id = r.id
		WHERE b.user_id = $1 ORDER BY b.created_at DESC`, userID)
	if err != nil {
		log.Printf("Ошибка запроса бронирований: %v", err)
		http.Error(w, "Ошибка загрузки бронирований", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var bookings []Booking
	for rows.Next() {
		var b Booking
		var ct time.Time
		err := rows.Scan(&b.ID, &b.CheckIn, &b.CheckOut, &b.Guests, &b.Status, &ct, &b.RoomName, &b.Price)
		if err != nil {
			continue
		}
		b.CreatedAt = ct.Format("02.01.2006 15:04")
		b.UserName = username
		bookings = append(bookings, b)
	}

	data := struct {
		Username string
		Bookings []Booking
	}{
		Username: username,
		Bookings: bookings,
	}

	if err := tmpl.ExecuteTemplate(w, "user_dashboard.html", data); err != nil {
		log.Printf("Ошибка выполнения шаблона user_dashboard.html: %v", err)
		http.Error(w, "Ошибка загрузки страницы", http.StatusInternalServerError)
	}
}

func managerDashboardHandler(w http.ResponseWriter, r *http.Request) {
	rows, err := db.Query(`
		SELECT b.id, b.check_in, b.check_out, b.guests, b.status, b.created_at, r.name, u.username
		FROM bookings b
		JOIN rooms r ON b.room_id = r.id
		JOIN users u ON b.user_id = u.id
		ORDER BY b.created_at DESC`)
	if err != nil {
		log.Printf("Ошибка запроса всех бронирований: %v", err)
		http.Error(w, "Ошибка загрузки бронирований", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var bookings []Booking
	for rows.Next() {
		var b Booking
		var ct time.Time
		err := rows.Scan(&b.ID, &b.CheckIn, &b.CheckOut, &b.Guests, &b.Status, &ct, &b.RoomName, &b.UserName)
		if err != nil {
			continue
		}
		b.CreatedAt = ct.Format("02.01.2006 15:04")
		bookings = append(bookings, b)
	}

	var total, confirmed, pending int
	db.QueryRow("SELECT COUNT(*) FROM bookings").Scan(&total)
	db.QueryRow("SELECT COUNT(*) FROM bookings WHERE status='confirmed'").Scan(&confirmed)
	db.QueryRow("SELECT COUNT(*) FROM bookings WHERE status='pending'").Scan(&pending)

	data := struct {
		Bookings          []Booking
		TotalBookings     int
		ConfirmedBookings int
		PendingBookings   int
	}{
		Bookings:          bookings,
		TotalBookings:     total,
		ConfirmedBookings: confirmed,
		PendingBookings:   pending,
	}

	if err := tmpl.ExecuteTemplate(w, "manager_dashboard.html", data); err != nil {
		log.Printf("Ошибка выполнения шаблона manager_dashboard.html: %v", err)
		http.Error(w, "Ошибка загрузки страницы", http.StatusInternalServerError)
	}
}

func roomsHandler(w http.ResponseWriter, r *http.Request) {
	session, _ := store.Get(r, "session")
	isAuth, _ := session.Values["authenticated"].(bool)
	role, _ := session.Values["role"].(string)

	rows, err := db.Query("SELECT id, name, description, price, capacity FROM rooms ORDER BY price")
	if err != nil {
		log.Printf("Ошибка запроса номеров: %v", err)
		http.Error(w, "Ошибка загрузки номеров", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var rooms []Room
	for rows.Next() {
		var r Room
		err := rows.Scan(&r.ID, &r.Name, &r.Description, &r.Price, &r.Capacity)
		if err != nil {
			continue
		}
		rooms = append(rooms, r)
	}

	data := struct {
		Rooms           []Room
		IsAuthenticated bool
		Role            string
	}{
		Rooms:           rooms,
		IsAuthenticated: isAuth,
		Role:            role,
	}

	if err := tmpl.ExecuteTemplate(w, "rooms.html", data); err != nil {
		log.Printf("Ошибка выполнения шаблона rooms.html: %v", err)
		http.Error(w, "Ошибка загрузки страницы", http.StatusInternalServerError)
	}
}

func bookFormHandler(w http.ResponseWriter, r *http.Request) {
	log.Println("bookFormHandler вызван")

	session, _ := store.Get(r, "session")
	isAuth, _ := session.Values["authenticated"].(bool)
	role, _ := session.Values["role"].(string)

	roomID := r.URL.Query().Get("id")
	log.Printf("roomID = %s", roomID)

	if roomID == "" {
		log.Println("roomID пустой, перенаправление на /rooms")
		http.Redirect(w, r, "/rooms", http.StatusSeeOther)
		return
	}

	var room Room
	err := db.QueryRow("SELECT id, name, description, price, capacity FROM rooms WHERE id = $1", roomID).
		Scan(&room.ID, &room.Name, &room.Description, &room.Price, &room.Capacity)
	if err != nil {
		log.Printf("Номер с ID=%s не найден в БД: %v", roomID, err)
		http.Error(w, "Номер не найден", http.StatusNotFound)
		return
	}

	log.Printf("Найден номер: %s (ID=%d)", room.Name, room.ID)

	data := struct {
		Title           string
		IsAuthenticated bool
		Role            string
		Room            Room
	}{
		Title:           "Бронирование номера",
		IsAuthenticated: isAuth,
		Role:            role,
		Room:            room,
	}

	if err := tmpl.ExecuteTemplate(w, "base.html", data); err != nil {
		log.Printf("Ошибка выполнения шаблона book.html через base.html: %v", err)
		http.Error(w, "Ошибка загрузки страницы", http.StatusInternalServerError)
	}
}

func bookRoomHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	s, _ := store.Get(r, "session")
	userID, ok := s.Values["user_id"].(int)
	if !ok {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}

	roomID := r.FormValue("room_id")
	checkIn := r.FormValue("check_in")
	checkOut := r.FormValue("check_out")
	guests := r.FormValue("guests")

	log.Printf("Бронирование: userID=%d, roomID=%s, checkIn=%s, checkOut=%s, guests=%s",
		userID, roomID, checkIn, checkOut, guests)

	if roomID == "" || checkIn == "" || checkOut == "" || guests == "" {
		log.Println("Ошибка: не все поля заполнены")
		http.Error(w, "Все поля обязательны", http.StatusBadRequest)
		return
	}

	checkInTime, err1 := time.Parse("2006-01-02", checkIn)
	checkOutTime, err2 := time.Parse("2006-01-02", checkOut)
	if err1 != nil || err2 != nil {
		log.Printf("Ошибка парсинга дат: checkIn=%s, checkOut=%s", checkIn, checkOut)
		http.Error(w, "Неверный формат даты", http.StatusBadRequest)
		return
	}
	if !checkOutTime.After(checkInTime) {
		log.Println("Ошибка: дата выезда не позже даты заезда")
		http.Error(w, "Дата выезда должна быть позже даты заезда", http.StatusBadRequest)
		return
	}

	var roomName string
	var roomPrice float64
	err := db.QueryRow("SELECT name, price FROM rooms WHERE id = $1", roomID).Scan(&roomName, &roomPrice)
	if err != nil {
		log.Printf("Комната с ID=%s не найдена", roomID)
		http.Error(w, "Комната не найдена", http.StatusBadRequest)
		return
	}

	_, err = db.Exec(`
		INSERT INTO bookings (user_id, room_id, check_in, check_out, guests, status)
		VALUES ($1, $2, $3, $4, $5, 'pending')
	`, userID, roomID, checkIn, checkOut, guests)
	if err != nil {
		log.Printf("Ошибка вставки в БД: %v", err)
		http.Error(w, "Ошибка бронирования: "+err.Error(), http.StatusInternalServerError)
		return
	}

	log.Println("Бронирование успешно создано!")
	http.Redirect(w, r, "/dashboard", http.StatusSeeOther)
}

func cancelBookingHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method == "POST" {
		s, _ := store.Get(r, "session")
		userID := s.Values["user_id"].(int)
		bookingID := r.FormValue("booking_id")

		var owner int
		db.QueryRow("SELECT user_id FROM bookings WHERE id = $1", bookingID).Scan(&owner)
		if owner != userID {
			http.Error(w, "Доступ запрещен", 403)
			return
		}

		db.Exec("UPDATE bookings SET status = 'cancelled' WHERE id = $1", bookingID)
		http.Redirect(w, r, "/dashboard", http.StatusSeeOther)
	}
}

func updateBookingHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method == "POST" {
		bookingID := r.FormValue("booking_id")
		status := r.FormValue("status")
		db.Exec("UPDATE bookings SET status = $1 WHERE id = $2", status, bookingID)
		http.Redirect(w, r, "/manager", http.StatusSeeOther)
	}
}