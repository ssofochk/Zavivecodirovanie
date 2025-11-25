CREATE TABLE IF NOT EXISTS transactions (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id uuid NOT NULL,
    amount int NOT NULL CHECK (amount > 0),
    type varchar(20) NOT NULL CHECK (type IN ('deposit', 'withdrawal', 'reserve', 'commit', 'rollback')),
    status varchar(20) NOT NULL DEFAULT 'completed' CHECK (status IN ('completed', 'reserved', 'cancelled')),
    expires_at timestamp(0) with time zone,
    created_at timestamp(0) with time zone NOT NULL DEFAULT NOW(),
    reservation_id uuid,
    remaining_amount int NOT NULL DEFAULT 0,
    CONSTRAINT check_remaining_amount CHECK (remaining_amount >= 0 AND remaining_amount <= amount)
);

CREATE INDEX idx_transactions_user_id ON transactions(user_id);
CREATE INDEX idx_transactions_expires_at ON transactions(expires_at) WHERE expires_at IS NOT NULL AND status = 'completed';
CREATE INDEX idx_transactions_user_status ON transactions(user_id, status);
CREATE INDEX idx_transactions_reservation_id ON transactions(reservation_id) WHERE reservation_id IS NOT NULL;
