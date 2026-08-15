-- esp_writing 4 小表数据迁移(TiDB→CRDB,显式保留源 id)
-- 2026-08-15 #140 全服务切 CRDB:TiDB 关停前不丢 writing_plans/progress/templates/essays 真实数据
SET DATABASE = esp_writing;

INSERT INTO writing_plans (id, user_id, weak_points_json, plan_json, status, created_at, updated_at) VALUES
(3170534137668829185, 1, '["kp-2138", "kp-3288", "kp-3984", "kp-7015"]'::jsonb,
'【写作能力提升方案(Mock)】
阶段1(1-2周):夯实语法——每日 1 篇改错练习,聚焦主谓一致/时态。
阶段2(3-4周):段落连贯——学习 topic sentence + 过渡词,每两天写 1 段。
阶段3(5-6周):词汇升级——替换 20 个高频词为学术词;每周完成 1 篇限时作文。
阶段4(7-8周):全真模拟——按考试时间完成整套写作,结合批改迭代。
', 'active', '2026-08-05 14:06:35.827+00', '2026-08-05 14:06:35.827+00'),
(7205759403792793602, 1, '["kp-2138", "kp-3288", "kp-3984", "kp-7015"]'::jsonb,
'【写作能力提升方案(Mock)】
阶段1(1-2周):夯实语法——每日 1 篇改错练习,聚焦主谓一致/时态。
阶段2(3-4周):段落连贯——学习 topic sentence + 过渡词,每两天写 1 段。
阶段3(5-6周):词汇升级——替换 20 个高频词为学术词;每周完成 1 篇限时作文。
阶段4(7-8周):全真模拟——按考试时间完成整套写作,结合批改迭代。
', 'active', '2026-08-05 22:15:19.537+00', '2026-08-05 22:15:19.537+00');

INSERT INTO writing_progress (id, user_id, plan_id, task_id, completed, score, created_at) VALUES
(576460752303423489,  1, 3170534137668829185, 't1',           1, 85.0, '2026-08-05 14:17:30.043+00'),
(1729382256910270466, 1, 7205759403792793602, 'stage1-task1', 1, 85.0, '2026-08-05 22:15:52.484+00');

INSERT INTO writing_templates (id, category, title, structure, difficulty, created_at) VALUES
(8646911284551352321, 'argumentative', '议论文标准五段式', '引言(背景+论点) | 正文段1 | 正文段2 | 正文段3(反驳) | 结论', 1, '2026-08-05 13:49:49.150+00'),
(8646911284551352322, 'argumentative', '议论文让步转折式', '引言 | 让步段 | 转折段 | 立论段 | 结论', 2, '2026-08-05 13:49:49.150+00'),
(8646911284551352323, 'letter', '正式信函(投诉/申请)', '称呼 | 开头(目的) | 主体1(情况) | 主体2(诉求) | 结尾署名', 1, '2026-08-05 13:49:49.150+00'),
(8646911284551352324, 'letter', 'informal letter 友人信', '称呼 | 寒暄+近况 | 主体(事由) | 结尾祝愿', 1, '2026-08-05 13:49:49.150+00'),
(8646911284551352325, 'chart', '图表作文(柱状/折线)', '引言(总览) | 主体1(主要趋势) | 主体2(对比) | 结论', 2, '2026-08-05 13:49:49.150+00'),
(8646911284551352326, 'chart', '饼图作文', '引言(总览) | 主体1(最大占比) | 主体2(其余构成) | 结论', 2, '2026-08-05 13:49:49.150+00'),
(8646911284551352327, 'narrative', '记叙文(事件)', '背景设定 | 冲突发生 | 高潮 | 解决 | 反思', 1, '2026-08-05 13:49:49.150+00'),
(8646911284551352328, 'narrative', '记叙文(人物)', '人物引入 | 事件触发 | 转变 | 结局 | 主题升华', 2, '2026-08-05 13:49:49.150+00');

INSERT INTO writing_essays (id, category, title, content, difficulty, word_count, created_at) VALUES
(8646911284551352321, 'argumentative', 'Should University Education Be Free?', 'University education should be free because it promotes social mobility and economic growth. When tuition is a barrier, talented students from low-income families are excluded. Free public universities ensure merit rather than wealth determines opportunity. Critics argue costs are prohibitive, yet long-term tax revenue from higher-earning graduates offsets initial investment. Therefore free higher education is both just and economically rational.', 1, 60, '2026-08-05 13:49:49.409+00'),
(8646911284551352322, 'letter', 'Complaint about a Defective Product', 'Dear Customer Service, I am writing to report a defect in the laptop I purchased on May 1st. The screen flickers after twenty minutes of use, making it unusable for work. I request a full refund or a replacement under the warranty. I have attached the receipt and a video of the issue. I look forward to your prompt response. Yours sincerely, J. Gong', 1, 64, '2026-08-05 13:49:49.409+00'),
(8646911284551352323, 'chart', 'Energy Consumption Trends 2010-2020', 'The chart illustrates energy consumption from 2010 to 2020. Overall fossil fuel use declined steadily while renewable energy rose. Coal dropped most sharply from 40 percent to 22 percent. Solar and wind surged reaching 28 percent by 2020. Natural gas remained relatively stable. The data signals a structural transition toward cleaner energy.', 2, 52, '2026-08-05 13:49:49.409+00'),
(8646911284551352324, 'narrative', 'The Day I Moved to a New City', 'The morning was grey when we arrived. Boxes stacked the entryway of an unfamiliar apartment. I stepped onto the balcony and watched strangers hurry below. Fear gripped me then. But by evening a neighbor knocked with warm soup and the city no longer felt foreign. That small kindness taught me that belonging begins with a single welcome.', 1, 57, '2026-08-05 13:49:49.409+00');
