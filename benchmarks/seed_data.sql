-- Seed Data for GraphPost Benchmarks
-- Run: psql graphpost_bench < benchmarks/seed_data.sql
-- Note: This may take a few minutes to complete

-- Set work_mem for faster inserts
SET work_mem = '256MB';

-- Disable constraints temporarily for faster inserts
SET session_replication_role = 'replica';

-- ============================================================================
-- Generate 1 million users
-- ============================================================================
DO $$
DECLARE
    batch_size INTEGER := 10000;
    total_users INTEGER := 1000000;
    i INTEGER := 0;
    status_options TEXT[] := ARRAY['active', 'inactive', 'pending', 'suspended'];
    role_options TEXT[] := ARRAY['user', 'moderator', 'admin', 'guest'];
BEGIN
    RAISE NOTICE 'Starting user generation...';

    WHILE i < total_users LOOP
        INSERT INTO users (name, email, age, status, role, created_at, metadata)
        SELECT
            'User ' || (i + g),
            'user' || (i + g) || '@example.com',
            18 + (random() * 60)::INTEGER,
            status_options[1 + (random() * 3)::INTEGER],
            role_options[1 + (random() * 3)::INTEGER],
            NOW() - (random() * INTERVAL '730 days'),
            jsonb_build_object(
                'preferences', jsonb_build_object(
                    'theme', CASE WHEN random() > 0.5 THEN 'dark' ELSE 'light' END,
                    'language', CASE WHEN random() > 0.5 THEN 'en' ELSE 'es' END
                ),
                'login_count', (random() * 100)::INTEGER
            )
        FROM generate_series(1, batch_size) g;

        i := i + batch_size;

        IF i % 100000 = 0 THEN
            RAISE NOTICE 'Users created: %', i;
        END IF;
    END LOOP;

    RAISE NOTICE 'User generation complete: % users', total_users;
END $$;

-- ============================================================================
-- Generate 100,000 products
-- ============================================================================
DO $$
DECLARE
    categories TEXT[] := ARRAY['Electronics', 'Clothing', 'Books', 'Home', 'Sports', 'Food', 'Toys', 'Health', 'Beauty', 'Automotive'];
BEGIN
    RAISE NOTICE 'Starting product generation...';

    INSERT INTO products (name, description, price, category, stock_quantity, is_active, created_at)
    SELECT
        'Product ' || g,
        'Description for product ' || g || '. This is a high-quality item.',
        (random() * 1000 + 1)::DECIMAL(10, 2),
        categories[1 + (random() * 9)::INTEGER],
        (random() * 1000)::INTEGER,
        random() > 0.1,
        NOW() - (random() * INTERVAL '365 days')
    FROM generate_series(1, 100000) g;

    RAISE NOTICE 'Product generation complete: 100000 products';
END $$;

-- ============================================================================
-- Generate 5 million orders
-- ============================================================================
DO $$
DECLARE
    batch_size INTEGER := 50000;
    total_orders INTEGER := 5000000;
    max_user_id INTEGER;
    i INTEGER := 0;
    status_options TEXT[] := ARRAY['pending', 'processing', 'shipped', 'delivered', 'completed', 'cancelled'];
BEGIN
    SELECT MAX(id) INTO max_user_id FROM users;

    RAISE NOTICE 'Starting order generation (max_user_id: %)...', max_user_id;

    WHILE i < total_orders LOOP
        INSERT INTO orders (user_id, status, total, shipping_address, created_at)
        SELECT
            1 + (random() * (max_user_id - 1))::INTEGER,
            status_options[1 + (random() * 5)::INTEGER],
            (random() * 500 + 10)::DECIMAL(10, 2),
            'Address ' || (i + g) || ', City, Country',
            NOW() - (random() * INTERVAL '365 days')
        FROM generate_series(1, batch_size) g;

        i := i + batch_size;

        IF i % 500000 = 0 THEN
            RAISE NOTICE 'Orders created: %', i;
        END IF;
    END LOOP;

    RAISE NOTICE 'Order generation complete: % orders', total_orders;
END $$;

-- ============================================================================
-- Generate order items (2-5 items per order, ~15 million items)
-- ============================================================================
DO $$
DECLARE
    batch_size INTEGER := 100000;
    max_order_id INTEGER;
    max_product_id INTEGER;
    current_batch INTEGER := 0;
    items_per_order INTEGER;
BEGIN
    SELECT MAX(id) INTO max_order_id FROM orders;
    SELECT MAX(id) INTO max_product_id FROM products;

    RAISE NOTICE 'Starting order items generation...';

    -- Process orders in batches
    FOR current_batch IN 0..((max_order_id / batch_size)::INTEGER) LOOP
        INSERT INTO order_items (order_id, product_id, product_name, quantity, unit_price, total_price, created_at)
        SELECT
            o.id,
            p.id,
            p.name,
            (1 + (random() * 4)::INTEGER),
            p.price,
            p.price * (1 + (random() * 4)::INTEGER),
            o.created_at
        FROM (
            SELECT id, created_at
            FROM orders
            WHERE id > current_batch * batch_size
              AND id <= (current_batch + 1) * batch_size
        ) o
        CROSS JOIN LATERAL (
            SELECT id, name, price
            FROM products
            ORDER BY random()
            LIMIT 2 + (random() * 3)::INTEGER
        ) p;

        IF current_batch % 10 = 0 THEN
            RAISE NOTICE 'Order items batch processed: %', current_batch;
        END IF;
    END LOOP;

    RAISE NOTICE 'Order items generation complete';
END $$;

-- Re-enable constraints
SET session_replication_role = 'origin';

-- Update order totals based on items
UPDATE orders o
SET total = (
    SELECT COALESCE(SUM(total_price), 0)
    FROM order_items oi
    WHERE oi.order_id = o.id
);

-- ============================================================================
-- Create additional indexes for benchmark queries
-- ============================================================================
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_users_composite
    ON users(status, age, created_at);

CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_orders_composite
    ON orders(status, created_at, total);

-- ============================================================================
-- Analyze tables for optimal query planning
-- ============================================================================
ANALYZE users;
ANALYZE products;
ANALYZE orders;
ANALYZE order_items;

-- ============================================================================
-- Display final counts
-- ============================================================================
SELECT 'users' as table_name, COUNT(*) as row_count FROM users
UNION ALL
SELECT 'products', COUNT(*) FROM products
UNION ALL
SELECT 'orders', COUNT(*) FROM orders
UNION ALL
SELECT 'order_items', COUNT(*) FROM order_items;

-- Display table sizes
SELECT
    tablename,
    pg_size_pretty(pg_total_relation_size('public.' || tablename)) as total_size,
    pg_size_pretty(pg_relation_size('public.' || tablename)) as table_size,
    pg_size_pretty(pg_indexes_size('public.' || tablename)) as index_size
FROM pg_tables
WHERE schemaname = 'public'
ORDER BY pg_total_relation_size('public.' || tablename) DESC;
