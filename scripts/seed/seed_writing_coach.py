"""灌 writing-coach 基础数据:写作模板 + 范文。

用法(集群内 pymysql pod 或本地):
  python scripts/seed/seed_writing_coach.py

走 tidb-primary 写主库。幂等:先 DELETE 再 INSERT(小数据量)。
"""
from __future__ import annotations
import sys, os

# 复用 db_query.sh 的 pymysql pod 方式:这里直接用 pymysql 连 tidb-primary
import pymysql

HOST = os.getenv("WRITE_DB_HOST", "tidb-primary")
USER = os.getenv("WRITE_DB_USER", "root")
DB = "esp"

TEMPLATES = [
    ("argumentative", "议论文标准五段式", "引言(背景+论点) | 正文段1(分论点1+论据) | 正文段2(分论点2+论据) | 正文段3(反驳+回应) | 结论(重申论点+升华)", 1),
    ("argumentative", "议论文让步转折式", "引言 | 让步段(承认对立观点合理处) | 转折段(指出其局限) | 立论段(我的观点+论据) | 结论", 2),
    ("letter", "正式信函(投诉/申请)", "称呼 | 开头(写信目的) | 主体段1(情况说明) | 主体段2(诉求/期望) | 结尾礼辞+署名", 1),
    ("letter", "informal letter 友人信", "称呼 | 寒暄+近况 | 主体(写信事由/邀请) | 结尾祝愿+署名", 1),
    ("chart", "图表作文(柱状/折线)", "引言(改写题目+总览) | 主体段1(主要趋势/极值) | 主体段2(对比/细节) | 结论(归纳)", 2),
    ("chart", "饼图作文", "引言(总览) | 主体段1(最大占比+原因) | 主体段2(其余构成+对比) | 结论", 2),
    ("narrative", "记叙文(事件)", "背景设定 | 冲突发生 | 高潮 | 解决 | 反思", 1),
    ("narrative", "记叙文(人物)", "人物引入 | 事件触发 | 转变 | 结局 | 主题升华", 2),
]

ESSAYS = [
    ("argumentative", "Should University Education Be Free?",
     "University education should be free because it promotes social mobility and economic growth. "
     "When tuition is a barrier, talented students from low-income families are excluded. "
     "Free public universities, funded by progressive taxation, ensure merit rather than wealth determines opportunity. "
     "Critics argue costs are prohibitive, yet the long-term tax revenue from higher-earning graduates offsets initial investment. "
     "Therefore, free higher education is both just and economically rational.", 1),
    ("letter", "Complaint about a Defective Product",
     "Dear Customer Service, I am writing to report a defect in the laptop I purchased on May 1st. "
     "The screen flickers after twenty minutes of use, making it unusable for work. "
     "I request a full refund or a replacement under the warranty. "
     "I have attached the receipt and a video of the issue. I look forward to your prompt response. Yours sincerely, J. Gong", 1),
    ("chart", "Energy Consumption Trends 2010-2020",
     "The chart illustrates energy consumption from 2010 to 2020. Overall, fossil fuel use declined steadily while renewable energy rose. "
     "Coal dropped most sharply, from 40% to 22%, reflecting policy shifts. Solar and wind surged, together reaching 28% by 2020. "
     "Natural gas remained relatively stable. The data signals a structural transition toward cleaner energy.", 2),
    ("narrative", "The Day I Moved to a New City",
     "The morning was grey when we arrived. Boxes stacked the entryway of an unfamiliar apartment. "
     "I stepped onto the balcony and watched strangers hurry below. Fear gripped me then. "
     "But by evening, a neighbor knocked with warm soup, and the city no longer felt foreign. "
     "That small kindness taught me that belonging begins with a single welcome.", 1),
]


def main() -> int:
    conn = pymysql.connect(host=HOST, user=USER, database=DB, autocommit=False,
                           connect_timeout=30, read_timeout=60, write_timeout=60)
    try:
        with conn.cursor() as cur:
            cur.execute("DELETE FROM writing_templates")
            cur.execute("DELETE FROM writing_essays")
            cur.executemany(
                "INSERT INTO writing_templates (category, title, structure, difficulty) VALUES (%s,%s,%s,%s)",
                TEMPLATES,
            )
            cur.executemany(
                "INSERT INTO writing_essays (category, title, content, difficulty, word_count) VALUES (%s,%s,%s,%s,%s)",
                [(c, t, content, d, len(content.split())) for (c, t, content, d) in ESSAYS],
            )
        conn.commit()
        with conn.cursor() as cur:
            cur.execute("SELECT COUNT(*) FROM writing_templates")
            t = cur.fetchone()[0]
            cur.execute("SELECT COUNT(*) FROM writing_essays")
            e = cur.fetchone()[0]
        print(f"OK templates={t} essays={e}")
        return 0
    finally:
        conn.close()


if __name__ == "__main__":
    sys.exit(main())
