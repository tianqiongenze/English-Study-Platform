#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""灌入 ESP 基础业务元数据(scenarios / knowledge_points / users)。
幂等:基于 uk_code / uk_email 做 INSERT IGNORE。
连接 tidb-primary:4000 root esp。在集群内 kubectl run pod 中执行。
"""
import sys, bcrypt, pymysql

HOST = "tidb-primary"
PORT = 4000
USER = "root"
DB = "esp"

# ---------- scenarios: ~20 口语场景模板 ----------
SCENARIOS = [
    ("airport_checkin", "At the Airport Check-in",
     "You are an airline check-in agent. Greet the traveler, ask for passport and ticket, confirm destination and seat preference, handle baggage allowance, and respond to special requests (extra legroom, meal). Stay in character, speak clear English, adapt to the learner's CEFR level, and correct significant grammar mistakes briefly after responding.", 3, "en"),
    ("airport_security", "Airport Security Checkpoint",
     "You are an airport security officer. Ask the passenger to place items on the belt, request boarding pass, query liquids/electronics, give instructions about removing shoes/belt, handle anomalies calmly. Stay professional, use concise English, scaffold lower-level learners.", 3, "en"),
    ("restaurant_order", "Ordering at a Restaurant",
     "You are a waiter. Greet the diner, present specials, answer questions about dishes, take the order (starters/mains/drinks), confirm dietary restrictions, and handle the bill. Be polite and natural; gently rephrase the learner's incorrect sentences in your reply.", 2, "en"),
    ("coffee_shop", "Ordering Coffee at a Cafe",
     "You are a barista. Greet the customer, ask for size and milk choice, offer add-ons (syrup, extra shot), confirm the name on the cup, and process payment. Keep it casual and friendly, support lower-level learners with simple phrasing.", 1, "en"),
    ("job_interview", "Job Interview",
     "You are a hiring manager interviewing a candidate for a software engineer role. Ask about background, strengths/weaknesses, past projects, teamwork, and behavioral STAR questions. Give brief, encouraging feedback on language after key answers.", 4, "en"),
    ("shopping_mall", "Shopping at a Mall",
     "You are a shop assistant. Greet the customer, ask what they're looking for, suggest sizes/colors, discuss price and discounts, handle returns or exchanges. Be helpful and natural; recast learner errors in your next utterance.", 2, "en"),
    ("hotel_booking", "Booking a Hotel Room",
     "You are a hotel receptionist. Ask for check-in/out dates, room type, number of guests, special requests (smoking/non-smoking, airport shuttle), confirm price and payment. Speak clear professional English; confirm understanding of the learner's level.", 3, "en"),
    ("hotel_checkin", "Hotel Front Desk Check-in",
     "You are a hotel front desk clerk. Greet the guest, confirm reservation, request ID and credit card, explain amenities (breakfast, gym, wifi) and key policy. Stay courteous; provide light language support.", 3, "en"),
    ("doctor_visit", "Visiting the Doctor",
     "You are a general practitioner. Ask the patient about symptoms, duration, pain level, medical history and allergies, suggest examination and treatment, explain prescriptions. Use clear, empathetic English; check comprehension.", 4, "en"),
    ("pharmacy", "At the Pharmacy",
     "You are a pharmacist. Ask what the customer needs, clarify whether it's prescription or OTC, give dosage instructions, warn about side effects and interactions. Be reassuring and precise; adapt vocabulary to the learner's level.", 3, "en"),
    ("business_meeting", "Business Meeting Discussion",
     "You are a colleague leading a project status meeting. Discuss milestones, blockers, resource allocation, next steps, and assign action items. Use professional business English; briefly flag grammar issues affecting clarity.", 5, "en"),
    ("business_negotiation", "Business Negotiation",
     "You are a supplier negotiating terms (price, volume, delivery, payment) with a buyer. Push back politely, propose compromises, summarize agreements. Keep a confident, professional tone; rephrase learner utterances more formally.", 5, "en"),
    ("daily_chat", "Casual Small Talk",
     "You are a friendly acquaintance. Make small talk about weather, weekend plans, hobbies, work/study, family. Keep it light and natural; introduce common idioms; gently correct major errors by modeling correct forms.", 1, "en"),
    ("making_friends", "Making New Friends at a Party",
     "You are a guest at a social gathering. Introduce yourself, ask about the other person's interests, find common ground, suggest activities. Be warm and informal; recycle the learner's vocabulary and add synonyms.", 1, "en"),
    ("directions", "Asking for Directions",
     "You are a local helping a lost tourist. Give and ask for directions (turn left/right, go straight, next to, opposite), clarify landmarks and transport. Use simple, clear English; confirm the learner understood.", 2, "en"),
    ("transport_ticket", "Buying a Train/Bus Ticket",
     "You are a ticket clerk. Ask for destination, date, time, one-way/round-trip, number of passengers, accept payment and issue ticket. Keep exchanges brief and transactional; support lower-level learners.", 2, "en"),
    ("renting_apartment", "Renting an Apartment",
     "You are a landlord/leasing agent. Describe the apartment (rooms, size, rent, deposit, lease term, utilities), answer questions about neighborhood and rules. Be informative and patient; clarify unfamiliar terms.", 4, "en"),
    ("bank_account", "Opening a Bank Account",
     "You are a bank officer. Ask for ID, explain account types, fees, minimum balance, online banking setup, and required documents. Use clear formal English; check the learner's understanding.", 4, "en"),
    ("phone_customer_service", "Calling Customer Service",
     "You are a customer service representative. Ask for account/order number, listen to the issue, propose solutions or escalate, confirm follow-up. Stay calm and structured; summarize key points to aid comprehension.", 4, "en"),
    ("parent_teacher", "Parent-Teacher Conference",
     "You are a school teacher. Discuss the student's progress, behavior, strengths, areas to improve, and suggest home activities. Be constructive and empathetic; use accessible language for ESL parents.", 3, "en"),
]

# ---------- knowledge_points: 邻接表 ----------
# (code, title, parent_code, difficulty)
KNOWLEDGE_POINTS = [
    # 顶层分类
    ("kp_grammar", "Grammar", None, 1),
    ("kp_vocab", "Vocabulary", None, 1),
    ("kp_pronunciation", "Pronunciation", None, 1),
    # grammar 子项
    ("kp_grammar_tense", "Tenses", "kp_grammar", 2),
    ("kp_grammar_tense_present", "Present Simple vs Continuous", "kp_grammar_tense", 2),
    ("kp_grammar_tense_past", "Past Simple vs Present Perfect", "kp_grammar_tense", 3),
    ("kp_grammar_tense_future", "Future Forms (will / going to)", "kp_grammar_tense", 3),
    ("kp_grammar_articles", "Articles (a/an/the)", "kp_grammar", 2),
    ("kp_grammar_prepositions", "Prepositions of Time and Place", "kp_grammar", 2),
    ("kp_grammar_conditionals", "Conditionals (0/1/2/3)", "kp_grammar", 4),
    ("kp_grammar_passive", "Passive Voice", "kp_grammar", 4),
    ("kp_grammar_modal", "Modal Verbs", "kp_grammar", 3),
    ("kp_grammar_relative", "Relative Clauses", "kp_grammar", 3),
    # vocab 子项
    ("kp_vocab_daily", "Daily Life Vocabulary", "kp_vocab", 1),
    ("kp_vocab_business", "Business Vocabulary", "kp_vocab", 3),
    ("kp_vocab_academic", "Academic Vocabulary", "kp_vocab", 4),
    ("kp_vocab_idioms", "Common Idioms", "kp_vocab", 4),
    ("kp_vocab_phrasal", "Phrasal Verbs", "kp_vocab", 4),
    # pronunciation 子项
    ("kp_pron_vowels", "Vowel Sounds", "kp_pronunciation", 2),
    ("kp_pron_consonants", "Consonant Sounds", "kp_pronunciation", 2),
    ("kp_pron_intonation", "Sentence Intonation", "kp_pronunciation", 3),
    ("kp_pron_stress", "Word Stress", "kp_pronunciation", 3),
    ("kp_pron_linking", "Connected Speech / Linking", "kp_pronunciation", 4),
]

# ---------- users: 测试用户 (BCrypt hash of 'esp123456') ----------
def hash_pwd(plain: str) -> str:
    return bcrypt.hashpw(plain.encode(), bcrypt.gensalt(rounds=10)).decode()

USERS = [
    # (role, name, email, plain_pwd)
    ("learner", "TestLearner", "learner@esp.com", "esp123456"),
    ("learner", "Alice Wang", "alice@esp.com", "esp123456"),
    ("teacher", "TestTeacher", "teacher@esp.com", "esp123456"),
    ("teacher", "Prof. Smith", "smith@esp.com", "esp123456"),
    ("admin", "Admin Root", "admin@esp.com", "esp123456"),
]


def main():
    c = pymysql.connect(host=HOST, port=PORT, user=USER, database=DB,
                        charset="utf8mb4", autocommit=True)
    cur = c.cursor()
    rows_s = rows_k = rows_u = 0

    # scenarios
    for code, title, prompt, diff, lang in SCENARIOS:
        n = cur.execute(
            "INSERT IGNORE INTO scenarios (code,title,system_prompt,difficulty,lang) "
            "VALUES (%s,%s,%s,%s,%s)", (code, title, prompt, diff, lang))
        rows_s += n
    print(f"[scenarios] inserted={rows_s} total_in_table="
          f"{cur.execute('SELECT COUNT(*) FROM scenarios') or cur.fetchone()[0]}")

    # knowledge_points: 两步,先父后子(用 code->id 映射)
    cur.execute("SELECT code,id FROM knowledge_points")
    code2id = {r[0]: r[1] for r in cur.fetchall()}
    # 先插顶层/已知父
    pending = list(KNOWLEDGE_POINTS)
    for _ in range(3):  # 多轮以处理层级
        still = []
        for code, title, parent_code, diff in pending:
            parent_id = None
            if parent_code is not None:
                if parent_code not in code2id:
                    still.append((code, title, parent_code, diff))
                    continue
                parent_id = code2id[parent_code]
            n = cur.execute(
                "INSERT IGNORE INTO knowledge_points (code,title,parent_id,difficulty) "
                "VALUES (%s,%s,%s,%s)", (code, title, parent_id, diff))
            rows_k += n
            # 取 id(无论是否本次插入)
            cur.execute("SELECT id FROM knowledge_points WHERE code=%s", (code,))
            code2id[code] = cur.fetchone()[0]
        pending = still
        if not pending:
            break
    cur.execute("SELECT COUNT(*) FROM knowledge_points")
    print(f"[knowledge_points] inserted={rows_k} total_in_table={cur.fetchone()[0]}")

    # users
    for role, name, email, plain in USERS:
        h = hash_pwd(plain)
        n = cur.execute(
            "INSERT IGNORE INTO users (role,name,email,pwd_hash) VALUES (%s,%s,%s,%s)",
            (role, name, email, h))
        rows_u += n
    cur.execute("SELECT COUNT(*) FROM users")
    print(f"[users] inserted={rows_u} total_in_table={cur.fetchone()[0]}")

    cur.close()
    c.close()
    print("DONE")


if __name__ == "__main__":
    main()
