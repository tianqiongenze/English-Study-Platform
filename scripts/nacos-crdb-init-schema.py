import psycopg2, sys

HOST, PORT, USER = '127.0.0.1', 26257, 'root'
SQL_FILE = '/Users/joelgong/nacos/nacos/conf/nacos-pg.sql'

def admin():
    c = psycopg2.connect(host=HOST, port=PORT, user=USER, dbname='defaultdb', connect_timeout=10)
    c.autocommit = True
    return c

# Step 1: CREATE DATABASE nacos (idempotent)
conn = admin(); cur = conn.cursor()
try:
    cur.execute("CREATE DATABASE IF NOT EXISTS nacos;")
    print("CREATE DATABASE nacos: OK")
except Exception as e:
    print("CREATE DATABASE FAIL:", repr(e)); sys.exit(1)
cur.close(); conn.close()

# Step 2: read & split schema into statements
with open(SQL_FILE) as f:
    raw = f.read()

# strip line comments (--) but keep block; nacos-pg.sql uses -- line comments only
stmts = []
buf = []
for line in raw.splitlines():
    # remove -- comment portion (naive but ok for this file)
    if '--' in line:
        line = line.split('--', 1)[0]
    buf.append(line)
full = '\n'.join(buf)
# split on semicolon
parts = [s.strip() for s in full.split(';')]
stmts = [s for s in parts if s]
print(f"parsed {len(stmts)} statements")

# Step 3: apply schema in nacos db
conn2 = psycopg2.connect(host=HOST, port=PORT, user=USER, dbname='nacos', connect_timeout=10)
conn2.autocommit = True
cur2 = conn2.cursor()
ok = 0; failed = []
for i, s in enumerate(stmts):
    try:
        cur2.execute(s)
        ok += 1
    except Exception as e:
        failed.append((i+1, s[:80].replace('\n',' '), repr(e)[:120]))
cur2.close(); conn2.close()
print(f"executed OK: {ok}/{len(stmts)}; failed: {len(failed)}")
for f in failed[:30]:
    print("  FAIL stmt#", f[0], "|", f[1], "->", f[2])
