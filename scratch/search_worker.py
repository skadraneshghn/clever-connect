import re

with open("scratch/splus_worker.js", "r", encoding="utf-8") as f:
    content = f.read()

# Let's search for "joinConferenceCall" call sites.
matches = re.finditer(r"joinConferenceCall\b", content, re.IGNORECASE)
for m in matches:
    start = max(0, m.start() - 200)
    end = min(len(content), m.end() + 200)
    print(f"Occurrence of joinConferenceCall at {m.start()}:")
    print(content[start:end])
    print("-" * 50)
