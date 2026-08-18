# CET-6 听力理解原创短脚本(esp_content 内容库 data 模块)
# LISTENINGS = [{title, cefr, script, question, options, correct, explanation}]
# 内容为原创(模拟听力场景脚本),版权安全。correct 为 0-based。
LISTENINGS = [
    {
        "title": "A Lecture on the History of Paper",
        "cefr": "B2",
        "script": "Professor: Today we'll discuss how paper changed civilisation. Before paper, people wrote on bamboo slips, silk, and animal skins. These materials were heavy, expensive, or both. Paper was invented in China around the first century, and the technique slowly spread westward. The process was improved over the centuries, and by the time of the industrial revolution, papermaking had become a large industry. Yet even today, our society still faces the challenge of balancing paper production with forest conservation.",
        "question": "What is the professor mainly discussing?",
        "options": ["How paper influenced the development of society", "The modern process of recycling paper", "Why bamboo is still used for writing", "The cost of paper in ancient times"],
        "correct": 0,
        "explanation": "讲座主题是纸如何改变文明——从早期书写材料到工业化再到环保挑战。",
    },
    {
        "title": "Announcement at the University Library",
        "cefr": "B2",
        "script": "Librarian: Attention students. Please note that the library will close early at 6 pm on Friday for scheduled maintenance. The reading rooms on the second floor will remain open, but book borrowing will be unavailable during the afternoon. All books due on Friday should be returned by 5 pm to avoid fines. The online catalogue will stay accessible throughout. We apologise for any inconvenience and thank you for your understanding.",
        "question": "What will NOT be available during Friday afternoon?",
        "options": ["Borrowing books from the library", "Using the second-floor reading rooms", "Accessing the online catalogue", "Returning books"],
        "correct": 0,
        "explanation": "公告明确周五下午图书借阅不可用,但二楼阅览室与在线目录仍开放。",
    },
    {
        "title": "A Radio Interview About Sleep",
        "cefr": "B2",
        "script": "Interviewer: Doctor, many of our listeners struggle to get enough sleep. What would you advise? Doctor: First, keep a consistent schedule, even on weekends. Second, avoid screens at least an hour before bed, because blue light suppresses melatonin, the hormone that regulates sleep. Third, keep your bedroom cool and dark. Interviewer: Does exercise help? Doctor: Yes, but time it carefully. Exercising too late in the evening can actually keep you awake because your body temperature stays elevated.",
        "question": "According to the doctor, why should people avoid screens before bed?",
        "options": ["Blue light reduces the hormone that regulates sleep", "Screens make the bedroom too bright", "Screens increase body temperature", "Blue light causes headaches"],
        "correct": 0,
        "explanation": "医生解释蓝光会抑制调节睡眠的褪黑素,所以睡前应避免屏幕。",
    },
    {
        "title": "A Conversation About a Research Project",
        "cefr": "B2",
        "script": "Tom: Sarah, have you decided on a topic for the research project? Sarah: I'm leaning towards the effects of social media on attention span. Tom: That's an interesting area. How do you plan to collect data? Sarah: I'll run a survey with about a hundred students and compare their self-reported screen time with their performance on a simple focus test. Tom: That sounds doable. Have you checked the ethics approval requirements? Sarah: Yes, I submitted the form this morning. I'm waiting for the confirmation now.",
        "question": "How will Sarah collect data for her project?",
        "options": ["By surveying students and testing their focus", "By interviewing professors", "By observing lectures", "By reading published papers only"],
        "correct": 0,
        "explanation": "Sarah 计划对约百名学生做问卷,并将屏幕时间与专注测试表现对比。",
    },
    {
        "title": "A News Report on Renewable Energy",
        "cefr": "B2",
        "script": "Reporter: In energy news today, a coastal city has announced plans to build a large offshore wind farm that could power over 200,000 homes. The project, scheduled to be completed in four years, is part of the city's goal to cut carbon emissions by half by 2035. Critics point out that construction costs are high and that fishing communities may be affected. Supporters, however, argue that the long-term economic and environmental gains outweigh these concerns. Local officials say public consultation will continue before final approval.",
        "question": "What is one criticism of the wind farm project?",
        "options": ["High construction costs and possible effects on fishing communities", "It will cut carbon emissions too quickly", "It will create too many jobs", "It has already been approved without consultation"],
        "correct": 0,
        "explanation": "批评者认为建设成本高,且渔区社区可能受到影响。",
    },
    {
        "title": "A Student Asking for an Extension",
        "cefr": "B2",
        "script": "Student: Professor, I was wondering if I could have an extension on the essay. Professor: The deadline is this Friday. What's the situation? Student: I've been unwell for the past few days, and I'm worried the work won't be complete to the standard expected. Professor: I understand. I can give you until next Tuesday, but only if you send me a short note from the clinic. Student: Thank you so much. I'll email the note today. Professor: Good. Let me know if anything changes.",
        "question": "What does the professor require for the extension?",
        "options": ["A short note from the clinic", "An email about the essay topic", "A completed draft by Friday", "A recommendation letter"],
        "correct": 0,
        "explanation": "教授同意延期至下周二,但要求提供诊所的简短证明。",
    },
]
