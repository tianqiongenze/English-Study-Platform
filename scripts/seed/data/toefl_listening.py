# TOEFL 听力理解原创学术脚本(esp_content 内容库 data 模块)
# LISTENINGS = [{title, cefr, script, question, options, correct, explanation}]
# 内容为原创(模拟托福校园/学术听力),版权安全。correct 为 0-based。
LISTENINGS = [
    {
        "title": "An Office Hour Discussion About Course Selection",
        "cefr": "B2",
        "script": "Student: Professor, I'm deciding between two courses for next semester: environmental economics and marine biology. Professor: Both are excellent. Which one fits your goals? Student: I'm interested in coastal management, but I lack a strong background in biology. Professor: Then environmental economics might be the safer choice. The marine biology course assumes knowledge of basic ecology. However, if you're willing to do extra reading, you could handle it. Student: I'd rather focus on depth this term. I'll go with the economics course. Professor: A sensible decision. Check the online syllabus and see you in September.",
        "question": "Why does the professor suggest the economics course?",
        "options": ["The student lacks a strong biology background", "The economics course is easier", "Marine biology is not offered next term", "The student dislikes coastal management"],
        "correct": 0,
        "explanation": "教授指出海洋生物课程以基础生态学为先修,而学生生物学基础较弱。",
    },
    {
        "title": "A Geology Lecture on Volcanoes",
        "cefr": "C1",
        "script": "Professor: Volcanoes are not randomly distributed across the globe. Most are found along the boundaries of tectonic plates, where the Earth's crust is either separating or colliding. When plates move apart, magma rises to fill the gap, forming ridge volcanoes. Where one plate slides beneath another, the melting rock rises to create chains of volcanoes, as we see around the Pacific Ring of Fire. Some volcanoes, however, form far from any boundary, above stationary hotspots in the mantle. These hotspot volcanoes, like the Hawaiian islands, are especially valuable for studying the slow movement of tectonic plates over millions of years.",
        "question": "Why are hotspot volcanoes considered especially valuable for research?",
        "options": ["They help scientists study plate movement over long periods", "They are the most violent type of volcano", "They contain rare minerals", "They only form on the ocean floor"],
        "correct": 0,
        "explanation": "热点火山远离板块边界,通过研究其链状分布可追踪板块数百万年的移动。",
    },
    {
        "title": "A Conversation About an Internship",
        "cefr": "B2",
        "script": "Student: I was offered an internship at a local research institute, but it conflicts with my statistics class. Advisor: When does the internship take place? Student: Tuesdays and Thursdays, from 9 to 12. My statistics class is Thursday morning. Advisor: You could ask the professor to let you switch to the online section. The institute position is valuable on your resume. Student: That's a good point. I'll email the professor today. Advisor: Do that soon. If the switch isn't possible, you may want to reconsider which experience matters more for your career.",
        "question": "What does the advisor suggest the student do first?",
        "options": ["Ask to switch to the online statistics section", "Cancel the internship", "Drop the statistics class", "Request a different internship"],
        "correct": 0,
        "explanation": "导师建议学生请教授允许转入在线统计班,以便兼顾实习。",
    },
    {
        "title": "A Lecture on Animal Communication",
        "cefr": "C1",
        "script": "Professor: Animal communication is far richer than simple signals. Consider the songs of birds. A song may advertise territory, attract a mate, or warn of danger, and listeners must interpret the same signal differently depending on context. Honeybees, remarkably, perform a dance that encodes the direction and distance of a food source. Researchers found that the tempo and angle of the dance correlate with distance and direction relative to the sun. What unites these examples is that communication requires both a sender and a receiver who share an understanding of the signal's meaning — a principle that applies to human language as well.",
        "question": "What do bird songs and honeybee dances have in common?",
        "options": ["Both require shared understanding between sender and receiver", "Both are used only to attract mates", "Both encode distance to food", "Both are learned in early life"],
        "correct": 0,
        "explanation": "共同点是交流需要发送者与接收者共享信号含义,这一原则同样适用于人类语言。",
    },
    {
        "title": "Registering for a Campus Event",
        "cefr": "B2",
        "script": "Clerk: Good morning, how can I help you? Student: I'd like to register for the career fair next week. Clerk: Certainly. Registration is free for students, but you need to submit your resume before Wednesday. Student: I'll upload it tonight. Is there a dress code? Clerk: Business casual is recommended. Also, bring several printed copies of your resume. Student: Will there be workshops? Clerk: Yes, two workshops on interview skills run in the morning. You can sign up at the entrance on the day of the event.",
        "question": "What must the student do before Wednesday?",
        "options": ["Submit their resume", "Buy business casual clothing", "Attend an interview workshop", "Print several copies of their resume"],
        "correct": 0,
        "explanation": "工作人员说明免费注册但须在周三前提交简历。",
    },
    {
        "title": "A Chemistry Lecture on Catalysts",
        "cefr": "C1",
        "script": "Professor: A catalyst is a substance that increases the rate of a chemical reaction without being consumed by it. This is possible because the catalyst provides an alternative pathway with a lower activation energy — the energy barrier that reactants must overcome. Consider industrial ammonia production. Without an iron catalyst, the reaction would require extreme temperatures and pressures, making the process too costly. With the catalyst, the reaction proceeds under far milder conditions. An important point: because the catalyst is not consumed, a small amount can convert a very large quantity of reactants, which is why catalysts are central to sustainable manufacturing.",
        "question": "How does a catalyst speed up a chemical reaction?",
        "options": ["By lowering the activation energy through an alternative pathway", "By increasing the temperature of the reaction", "By being consumed during the reaction", "By adding more reactants"],
        "correct": 0,
        "explanation": "催化剂提供更低活化能的替代路径,且自身不被消耗。",
    },
]
