package gotfo

type conn_t struct {
	fd *netFD
}

type tcp_conn_t struct {
	conn_t
}
