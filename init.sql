-- GraphPost Test Database Schema

-- Users table
CREATE TABLE IF NOT EXISTS users (
    id SERIAL PRIMARY KEY,
    name VARCHAR(255) NOT NULL,
    email VARCHAR(255) UNIQUE NOT NULL,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

-- Posts table
CREATE TABLE IF NOT EXISTS posts (
    id SERIAL PRIMARY KEY,
    title VARCHAR(255) NOT NULL,
    content TEXT,
    author_id INTEGER REFERENCES users(id) ON DELETE CASCADE,
    published BOOLEAN DEFAULT FALSE,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

-- Comments table
CREATE TABLE IF NOT EXISTS comments (
    id SERIAL PRIMARY KEY,
    content TEXT NOT NULL,
    post_id INTEGER REFERENCES posts(id) ON DELETE CASCADE,
    author_id INTEGER REFERENCES users(id) ON DELETE CASCADE,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

-- Categories table
CREATE TABLE IF NOT EXISTS categories (
    id SERIAL PRIMARY KEY,
    name VARCHAR(100) UNIQUE NOT NULL,
    description TEXT
);

-- Post-Category junction table
CREATE TABLE IF NOT EXISTS post_categories (
    post_id INTEGER REFERENCES posts(id) ON DELETE CASCADE,
    category_id INTEGER REFERENCES categories(id) ON DELETE CASCADE,
    PRIMARY KEY (post_id, category_id)
);

-- Insert sample data
INSERT INTO users (name, email) VALUES
    ('Alice Johnson', 'alice@example.com'),
    ('Bob Smith', 'bob@example.com'),
    ('Charlie Brown', 'charlie@example.com');

INSERT INTO categories (name, description) VALUES
    ('Technology', 'Tech-related posts'),
    ('Travel', 'Travel stories and tips'),
    ('Food', 'Recipes and restaurant reviews');

INSERT INTO posts (title, content, author_id, published) VALUES
    ('Getting Started with GraphQL', 'GraphQL is a query language for APIs...', 1, true),
    ('My Trip to Japan', 'Last summer, I visited Tokyo...', 2, true),
    ('Best Pizza in NYC', 'After trying 50 pizza places...', 3, true),
    ('Draft Post', 'This is a work in progress...', 1, false);

INSERT INTO comments (content, post_id, author_id) VALUES
    ('Great article!', 1, 2),
    ('Very helpful, thanks!', 1, 3),
    ('I want to visit Japan too!', 2, 1),
    ('Disagree with #3, try Joes!', 3, 2);

INSERT INTO post_categories (post_id, category_id) VALUES
    (1, 1),
    (2, 2),
    (3, 3);

-- Create indexes for better performance
CREATE INDEX idx_posts_author_id ON posts(author_id);
CREATE INDEX idx_comments_post_id ON comments(post_id);
CREATE INDEX idx_comments_author_id ON comments(author_id);
