-- 创建数据库
CREATE DATABASE IF NOT EXISTS matchengine DEFAULT CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;

USE matchengine;

-- 订单表
CREATE TABLE IF NOT EXISTS orders (
    id BIGINT UNSIGNED PRIMARY KEY,
    user_id BIGINT UNSIGNED NOT NULL,
    symbol VARCHAR(20) NOT NULL,
    side TINYINT NOT NULL COMMENT '1=buy, 2=sell',
    type TINYINT NOT NULL COMMENT '1=limit, 2=market, 3=stop_limit, 4=stop_market, 5=iceberg',
    status TINYINT NOT NULL COMMENT '1=new, 2=partially_filled, 3=filled, 4=cancelled, 5=expired, 6=rejected',
    price DECIMAL(30, 18) NOT NULL DEFAULT 0,
    amount DECIMAL(30, 18) NOT NULL DEFAULT 0,
    filled_amount DECIMAL(30, 18) NOT NULL DEFAULT 0,
    remain_amount DECIMAL(30, 18) NOT NULL DEFAULT 0,
    filled_total DECIMAL(30, 18) NOT NULL DEFAULT 0,
    fee DECIMAL(30, 18) NOT NULL DEFAULT 0,
    fee_asset VARCHAR(20) DEFAULT '',
    time_in_force TINYINT NOT NULL DEFAULT 1 COMMENT '1=GTC, 2=IOC, 3=FOK, 4=GTX',
    stop_price DECIMAL(30, 18) DEFAULT 0,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    INDEX idx_user_symbol (user_id, symbol),
    INDEX idx_symbol (symbol),
    INDEX idx_status (status),
    INDEX idx_created_at (created_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='订单表';

-- 成交表
CREATE TABLE IF NOT EXISTS trades (
    id BIGINT UNSIGNED PRIMARY KEY,
    taker_order_id BIGINT UNSIGNED NOT NULL,
    maker_order_id BIGINT UNSIGNED NOT NULL,
    user_id BIGINT UNSIGNED NOT NULL COMMENT 'taker user id',
    maker_user_id BIGINT UNSIGNED NOT NULL COMMENT 'maker user id',
    symbol VARCHAR(20) NOT NULL,
    price DECIMAL(30, 18) NOT NULL,
    amount DECIMAL(30, 18) NOT NULL,
    total DECIMAL(30, 18) NOT NULL,
    fee DECIMAL(30, 18) NOT NULL DEFAULT 0,
    fee_asset VARCHAR(20) DEFAULT '',
    side TINYINT NOT NULL COMMENT 'taker side',
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    INDEX idx_taker_order (taker_order_id),
    INDEX idx_maker_order (maker_order_id),
    INDEX idx_user (user_id),
    INDEX idx_maker_user (maker_user_id),
    INDEX idx_symbol (symbol),
    INDEX idx_time (created_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='成交表';

-- K线表
CREATE TABLE IF NOT EXISTS klines (
    id BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
    symbol VARCHAR(20) NOT NULL,
    open_time BIGINT NOT NULL COMMENT '开盘时间戳(秒)',
    close_time BIGINT NOT NULL COMMENT '收盘时间戳(秒)',
    open DECIMAL(30, 18) NOT NULL,
    high DECIMAL(30, 18) NOT NULL,
    low DECIMAL(30, 18) NOT NULL,
    close DECIMAL(30, 18) NOT NULL,
    volume DECIMAL(30, 18) NOT NULL DEFAULT 0,
    amount DECIMAL(30, 18) NOT NULL DEFAULT 0,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE KEY idx_symbol_time (symbol, open_time),
    INDEX idx_open_time (open_time)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='K线表';

-- 账户表
CREATE TABLE IF NOT EXISTS accounts (
    id BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
    user_id BIGINT UNSIGNED NOT NULL,
    asset VARCHAR(20) NOT NULL,
    available DECIMAL(30, 18) NOT NULL DEFAULT 0,
    frozen DECIMAL(30, 18) NOT NULL DEFAULT 0,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    UNIQUE KEY idx_user_asset (user_id, asset)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='账户表';

-- 资产流水表
CREATE TABLE IF NOT EXISTS account_history (
    id BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
    user_id BIGINT UNSIGNED NOT NULL,
    asset VARCHAR(20) NOT NULL,
    `change` DECIMAL(30, 18) NOT NULL COMMENT '变动数量，正数增加，负数减少',
    available_before DECIMAL(30, 18) NOT NULL,
    available_after DECIMAL(30, 18) NOT NULL,
    frozen_before DECIMAL(30, 18) NOT NULL,
    frozen_after DECIMAL(30, 18) NOT NULL,
    ref_type VARCHAR(20) NOT NULL COMMENT '关联类型: order, trade, deposit, withdraw',
    ref_id BIGINT UNSIGNED NOT NULL COMMENT '关联ID',
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    INDEX idx_user_asset (user_id, asset),
    INDEX idx_ref (ref_type, ref_id),
    INDEX idx_time (created_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='资产流水表';
