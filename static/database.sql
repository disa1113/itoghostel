-- Создание базы данных
CREATE DATABASE IF NOT EXISTS hostel_db;
USE hostel_db;

-- Таблица пользователей
CREATE TABLE IF NOT EXISTS users (
    id INT AUTO_INCREMENT PRIMARY KEY,
    username VARCHAR(50) UNIQUE NOT NULL,
    email VARCHAR(100) UNIQUE NOT NULL,
    password VARCHAR(255) NOT NULL,
    role VARCHAR(20) DEFAULT 'user',
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- Таблица номеров
CREATE TABLE IF NOT EXISTS rooms (
    id INT AUTO_INCREMENT PRIMARY KEY,
    name VARCHAR(100) NOT NULL,
    description TEXT,
    price DECIMAL(10,2) NOT NULL,
    capacity INT NOT NULL
);

-- Таблица бронирований
CREATE TABLE IF NOT EXISTS bookings (
    id INT AUTO_INCREMENT PRIMARY KEY,
    user_id INT NOT NULL,
    room_id INT NOT NULL,
    check_in DATE NOT NULL,
    check_out DATE NOT NULL,
    guests INT NOT NULL,
    status VARCHAR(20) DEFAULT 'pending',
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
    FOREIGN KEY (room_id) REFERENCES rooms(id) ON DELETE CASCADE
);

-- Вставка тестовых номеров
INSERT INTO rooms (name, description, price, capacity) VALUES 
('Эконом 6-местный', 'Бюджетный номер на 6 человек', 800, 6),
('Стандарт 4-местный', 'Комфортабельный номер на 4 человека', 1200, 4),
('Комфорт 2-местный', 'Уютный двухместный номер', 1800, 2),
('Делюкс 2-местный', 'Просторный двухместный номер', 2500, 2),
('Премиум 1-местный', 'Люкс для одного гостя', 3200, 1),
('Семейный люкс', 'Просторный семейный номер', 4500, 4);

-- Примечание: Пользователей добавляет приложение при первом запуске с хешированными паролями
-- Для добавления пользователя вручную используйте bcrypt хеш пароля
