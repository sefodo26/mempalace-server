"""
Synthetic knowledge base for the MemPalace benchmark.

A fictional company ("Aurelia Robotics") internal handbook, expressed as atomic
facts. Each fact is a self-contained sentence — the unit MemPalace stores as a
"drawer" / card. Every probe question maps to exactly one gold fact and has a
short, distinctive answer token so grading is deterministic (substring match),
not a fuzzy LLM judgement.

Design notes that make this a *real* retrieval test, not a lookup:
  * Questions are paraphrased away from the fact wording (no lexical give-away
    for pure vector search; some lexical overlap so full-text still contributes).
  * Every topic has near-duplicate distractor facts (same subject, different
    number/name) so top-1 retrieval actually has to discriminate.
  * Facts are grouped into wings/rooms so filtered search can be exercised too.
  * A couple of probes are multilingual (German) to exercise embeddinggemma's
    multilingual claim.

The corpus is intentionally hand-authored (not LLM-generated) so the gold
answers are known with certainty and the benchmark is reproducible.
"""

from dataclasses import dataclass, field


@dataclass
class Fact:
    id: str
    wing: str
    room: str
    text: str
    # session in which an agent would first "learn" this fact (1-based)
    session: int = 1


@dataclass
class Probe:
    question: str
    answer: str          # distinctive gold token, matched case-insensitively
    gold_id: str         # id of the Fact that answers it
    aliases: list = field(default_factory=list)  # acceptable alternative tokens


# ---------------------------------------------------------------------------
# Facts — atomic, each with a unique answer token. Grouped by wing/room.
# ---------------------------------------------------------------------------
FACTS = [
    # --- engineering / services ------------------------------------------
    Fact("orion-port", "engineering", "services",
         "The Orion service's staging database runs on host orion-db-7f3a and listens on TCP port 5442.", 1),
    Fact("orion-owner", "engineering", "services",
         "The Orion recommendation service is owned by the Perception team and its tech lead is Priya Nandakumar.", 1),
    Fact("vega-port", "engineering", "services",
         "The Vega billing service exposes its gRPC API on port 8931 in production.", 1),
    Fact("vega-lang", "engineering", "services",
         "The Vega billing service was rewritten from Python to Rust in the second quarter to cut tail latency.", 2),
    Fact("lyra-cache", "engineering", "services",
         "The Lyra search frontend caches query results in Redis with a time-to-live of 900 seconds.", 1),
    Fact("lyra-timeout", "engineering", "services",
         "The Lyra search frontend aborts any upstream request that exceeds a hard timeout of 250 milliseconds.", 2),

    # --- engineering / infra ---------------------------------------------
    Fact("k8s-version", "engineering", "infra",
         "Production Kubernetes clusters are pinned to version 1.29 and upgraded one minor version per quarter.", 1),
    Fact("region-primary", "engineering", "infra",
         "The primary production region is eu-central-1 in Frankfurt; the warm standby is eu-west-1 in Dublin.", 1),
    Fact("db-backup", "engineering", "infra",
         "PostgreSQL backups are taken every 6 hours and retained for 35 days in encrypted object storage.", 2),
    Fact("cdn-vendor", "engineering", "infra",
         "Static assets are served through the Fastly content delivery network under contract AUR-CDN-2231.", 1),
    Fact("secret-rotation", "engineering", "infra",
         "Service credentials are rotated automatically every 45 days by the Vault sidecar.", 3),

    # --- security --------------------------------------------------------
    Fact("mfa-policy", "security", "policy",
         "All employees must use hardware security keys for multi-factor authentication; TOTP apps are not permitted.", 1),
    Fact("pentest-cadence", "security", "policy",
         "An external penetration test is commissioned twice a year, in March and September.", 1),
    Fact("incident-sla", "security", "incident",
         "A Sev-1 security incident must be acknowledged within 15 minutes and a first status posted within 30 minutes.", 2),
    Fact("data-retention", "security", "policy",
         "Customer personal data is deleted 90 days after account closure unless a legal hold applies.", 1),
    Fact("bug-bounty", "security", "policy",
         "The bug bounty program pays a maximum of 12000 euros for a single critical remote-code-execution report.", 3),

    # --- hr / people -----------------------------------------------------
    Fact("pto-days", "hr", "benefits",
         "Full-time employees accrue 28 days of paid time off per year, plus public holidays.", 1),
    Fact("remote-policy", "hr", "benefits",
         "The company is remote-first; employees may work from any country in the EU without prior approval.", 1),
    Fact("learning-budget", "hr", "benefits",
         "Each engineer receives an annual learning budget of 1500 euros for courses, books and conferences.", 2),
    Fact("review-cycle", "hr", "process",
         "Performance reviews run on a calendar twice a year, closing on 30 June and 31 December.", 1),
    Fact("onboarding-buddy", "hr", "process",
         "Every new hire is assigned an onboarding buddy for their first 60 days.", 2),
    Fact("ceo-name", "hr", "people",
         "The chief executive officer of Aurelia Robotics is Dr. Helena Vasquez, who founded the company in 2016.", 1),

    # --- finance ---------------------------------------------------------
    Fact("fiscal-year", "finance", "accounting",
         "The company's fiscal year ends on 31 March.", 1),
    Fact("expense-limit", "finance", "policy",
         "Any expense above 2500 euros requires approval from a director before it is incurred.", 1),
    Fact("travel-class", "finance", "policy",
         "Economy class is the default for flights; business class is allowed only for flights longer than 8 hours.", 2),
    Fact("invoice-terms", "finance", "accounting",
         "Standard supplier invoices are paid on net-30 terms from the date of receipt.", 1),
    Fact("runway", "finance", "accounting",
         "As of the last board update the company held 22 months of cash runway.", 3),

    # --- product ---------------------------------------------------------
    Fact("flagship", "product", "roadmap",
         "The flagship product is the Aurelia Warehouse Pilot, an autonomous forklift control system.", 1),
    Fact("launch-date", "product", "roadmap",
         "The next major release, code-named Meridian, is scheduled to ship on 14 November.", 2),
    Fact("sla-uptime", "product", "sla",
         "The customer-facing uptime commitment in the enterprise contract is 99.95 percent per calendar month.", 1),
    Fact("support-hours", "product", "sla",
         "Premium support covers 24 hours a day on weekdays and 12 hours a day on weekends.", 2),
    Fact("pricing-tier", "product", "pricing",
         "The Growth pricing tier costs 4900 euros per month and includes up to 40 connected robots.", 3),

    # --- operations ------------------------------------------------------
    Fact("office-city", "operations", "facilities",
         "The company headquarters is located in Munich, with a hardware lab in Graz.", 1),
    Fact("standup-time", "operations", "process",
         "The engineering-wide daily standup is held at 09:45 Central European Time.", 1),
    Fact("release-day", "operations", "process",
         "Production deployments are frozen on Fridays; the regular release window is Tuesday afternoon.", 2),
    Fact("oncall-rotation", "operations", "process",
         "The on-call rotation is one week long and handed over every Monday at 10:00.", 2),
    Fact("vendor-laptop", "operations", "facilities",
         "Standard-issue developer laptops are the Framework 16, provisioned with disk encryption enabled.", 3),
]


# ---------------------------------------------------------------------------
# Probes — one gold fact each, paraphrased, deterministic answer token.
# ---------------------------------------------------------------------------
PROBES = [
    Probe("Which TCP port does Orion's staging database use?", "5442", "orion-port"),
    Probe("Who is the tech lead responsible for the Orion recommendation service?", "Priya", "orion-owner", ["Nandakumar"]),
    Probe("On what port is Vega's production gRPC API available?", "8931", "vega-port"),
    Probe("What programming language was the Vega billing service rewritten in?", "Rust", "vega-lang"),
    Probe("How long does Lyra keep cached search results before they expire?", "900", "lyra-cache"),
    Probe("What is the upstream request timeout for the Lyra search frontend?", "250", "lyra-timeout"),
    Probe("What Kubernetes version do the production clusters run?", "1.29", "k8s-version"),
    Probe("Where is the main production region hosted?", "Frankfurt", "region-primary", ["eu-central-1"]),
    Probe("How often are the PostgreSQL backups taken?", "6 hours", "db-backup", ["every 6"]),
    Probe("Which CDN provider delivers the static assets?", "Fastly", "cdn-vendor"),
    Probe("How frequently are service credentials rotated?", "45", "secret-rotation"),
    Probe("What kind of second factor is mandatory for staff sign-in?", "hardware security key", "mfa-policy", ["hardware"]),
    Probe("How many times per year is an external penetration test run?", "twice", "pentest-cadence", ["two"]),
    Probe("Within how many minutes must a Sev-1 security incident be acknowledged?", "15", "incident-sla"),
    Probe("How long is customer data kept after an account is closed?", "90", "data-retention"),
    Probe("What is the top payout of the bug bounty program for a critical RCE?", "12000", "bug-bounty", ["12,000"]),
    Probe("How many paid vacation days do full-time staff get each year?", "28", "pto-days"),
    Probe("Can employees work remotely from anywhere in the EU?", "remote-first", "remote-policy", ["any country"]),
    Probe("What is the yearly professional-development budget per engineer?", "1500", "learning-budget"),
    Probe("When do the two performance-review cycles close?", "30 June", "review-cycle", ["31 December"]),
    Probe("How long does a new hire keep their onboarding buddy?", "60", "onboarding-buddy"),
    Probe("Who runs Aurelia Robotics as chief executive?", "Helena Vasquez", "ceo-name", ["Vasquez"]),
    Probe("When does the company's fiscal year end?", "31 March", "fiscal-year", ["March"]),
    Probe("Above what amount does an expense need director approval?", "2500", "expense-limit"),
    Probe("For how long a flight is business class permitted?", "8 hours", "travel-class", ["longer than 8"]),
    Probe("What payment terms apply to standard supplier invoices?", "net-30", "invoice-terms", ["30"]),
    Probe("How many months of cash runway does the company have?", "22", "runway"),
    Probe("What is Aurelia's flagship product?", "Warehouse Pilot", "flagship", ["forklift"]),
    Probe("On what date does the Meridian release ship?", "14 November", "launch-date", ["November"]),
    Probe("What monthly uptime does the enterprise SLA promise?", "99.95", "sla-uptime"),
    Probe("What are the weekend hours for premium support?", "12", "support-hours"),
    Probe("How much does the Growth pricing tier cost per month?", "4900", "pricing-tier"),
    Probe("In which city is the company headquartered?", "Munich", "office-city"),
    Probe("At what time is the engineering standup held?", "09:45", "standup-time", ["9:45"]),
    Probe("Which weekday is the normal production release window?", "Tuesday", "release-day"),
    Probe("How long is one on-call shift in the rotation?", "one week", "oncall-rotation", ["week"]),
    Probe("Which laptop model is issued to developers?", "Framework 16", "vendor-laptop", ["Framework"]),
    # multilingual probes (German questions, same gold facts)
    Probe("Auf welchem Port lauscht die Staging-Datenbank von Orion?", "5442", "orion-port"),
    Probe("Wie viele bezahlte Urlaubstage bekommen Vollzeitangestellte pro Jahr?", "28", "pto-days"),
    Probe("Wer ist die Geschäftsführerin von Aurelia Robotics?", "Helena Vasquez", "ceo-name", ["Vasquez"]),
]


def facts_by_id():
    return {f.id: f for f in FACTS}


def facts_for_session(n: int):
    """Facts learned in session <= n."""
    return [f for f in FACTS if f.session <= n]


def probes_answerable_by_session(n: int):
    """Probes whose gold fact has been learned by session n."""
    learned = {f.id for f in facts_for_session(n)}
    return [p for p in PROBES if p.gold_id in learned]


NUM_SESSIONS = max(f.session for f in FACTS)


if __name__ == "__main__":
    # sanity: every probe references a real fact, every answer token is present
    ids = facts_by_id()
    problems = 0
    for p in PROBES:
        if p.gold_id not in ids:
            print(f"BAD gold_id: {p.gold_id}")
            problems += 1
            continue
        f = ids[p.gold_id]
        toks = [p.answer] + p.aliases
        if not any(t.lower() in f.text.lower() for t in toks):
            print(f"answer {toks} not found in gold fact {p.gold_id}: {f.text!r}")
            problems += 1
    print(f"{len(FACTS)} facts, {len(PROBES)} probes, {NUM_SESSIONS} sessions, {problems} problems")
