# Claude Code Prompts — HubLive Server Refactor

> **Hướng dẫn:** Copy từng prompt vào Claude Code theo thứ tự.
> Mỗi prompt kết thúc bằng verification — PHẢI pass hết trước khi sang prompt tiếp.
> Prompts được thiết kế để Claude Code có thể tự chạy lệnh, đọc code, sửa code, và test.

---

## PROMPT ROADMAP — Tổng quan 15 prompts

```
PHASE 0 — PREPARATION (không sửa production code)
  Prompt 1:  Audit & Baseline
             → Scan codebase structure, audit external imports, audit sentinel errors,
               audit servicefakes usage, tạo check-imports.sh script,
               tạo benchmark baseline report
             → Output: audit-report.md + scripts/check-imports.sh

PHASE 1 — EXTRACT STORE LAYER
  Prompt 2:  Create store/interfaces.go + move localstore + redisstore
             → Tạo pkg/store/, move files, update imports, add transition aliases,
               regenerate fakes, run tests
             → Output: pkg/store/ hoàn chỉnh, service/ đã bỏ store files

PHASE 2A — EXTRACT API HANDLERS
  Prompt 3:  Move auth middleware + all Twirp handlers to pkg/api/
             → Move auth.go, basic_auth.go, roomservice.go, egress.go, ingress.go,
               sip.go, agentservice.go, clientconfiguration_service.go
             → Output: pkg/api/ hoàn chỉnh, service/ đã bỏ API handler files

PHASE 2B — STABILIZE ROOMMANAGER INTERFACE
  Prompt 4:  Define SessionHandler interface + RoomManager interface
             → Tạo interface trong transport/interfaces.go,
               update rtcservice.go và signal.go dùng interface
             → Output: RoomManager depended via interface, not concrete

PHASE 3 — EXTRACT TRANSPORT LAYER
  Prompt 5:  Create transport registry + move websocket + whip handlers
             → Tạo pkg/transport/, TransportRegistry, move rtcservice.go → websocket/,
               signal.go → websocket/, whipservice.go → whip/
             → Update server.go dùng registry
             → Output: pkg/transport/ hoàn chỉnh

PHASE 4 — EXTRACT DOMAIN: TRACK & ROOM
  Prompt 6:  Create domain/track/ — move mediatrack, subscribedtrack, datatrack
             → Move files từ rtc/ sang domain/track/, tạo interfaces.go,
               add aliases trong rtc/types/
             → Output: pkg/domain/track/ hoàn chỉnh

  Prompt 7:  Create domain/room/ — move room.go, track_manager, auto_subscription
             → Move + split rtc/room.go, tạo SubscriptionPolicy interface,
               DefaultSubscriptionPolicy
             → Output: pkg/domain/room/ hoàn chỉnh

PHASE 5 — SPLIT PARTICIPANT GOD OBJECT
  Prompt 8:  Create domain/participant/shared.go + participant.go (core state)
             → Tạo SharedContext struct, SignalSender interface, core state extraction
             → Output: SharedContext + participant core

  Prompt 9:  Create publisher.go + subscriber.go + signal_handler.go
             → Split remaining 4700 lines into 3 focused files,
               wire PublisherState + SubscriberState vào ParticipantImpl
             → Output: participant split hoàn chỉnh

PHASE 6 — SPLIT WEBRTC TRANSPORT
  Prompt 10: Split rtc/transport.go → transport/webrtc/ (3 files)
             → connection.go, negotiation.go, ice.go
             → Move rtc/participant_sdp.go logic vào negotiation.go
             → Output: pkg/transport/webrtc/ hoàn chỉnh

PHASE 7 — AGENT DOMAIN + EXTENSION POINTS
  Prompt 11: Create domain/agent/ + formalize all extension interfaces
             → Move agent dispatch logic, tạo AgentDispatcher + WorkerRegistry interfaces,
               add RouterPlugin interface, add NullRouterPlugin
             → Output: 5 extension points hoàn chỉnh

  Prompt 12: Update Wire DI + register all extension points with defaults
             → Update service/wire.go cho tất cả packages mới,
               wire default implementations
             → Output: Wire compiles, tất cả extensions wired

PHASE 8 — CLEANUP & DOCUMENTATION
  Prompt 13: Remove transition aliases + dead code + orphan imports
             → Delete compat.go files, remove rtc/types aliases,
               regenerate all fakes, final import cleanup
             → Output: Zero aliases, zero dead code

  Prompt 14: Final verification + documentation
             → Run full test suite, benchmark comparison vs baseline,
               update README.md with new structure, commit decision log
             → Output: Ship-ready codebase

BONUS (optional)
  Prompt 15: Write extension example — custom SubscriptionPolicy
             → Implement a sample role-based SubscriptionPolicy as proof
               that extension points work
             → Output: Working example in examples/ directory
```

---

## NGUYÊN TẮC VIẾT PROMPT CHO CLAUDE CODE

1. **Mỗi prompt có CONTEXT block** — nhắc Claude Code đang ở đâu trong plan
2. **Mỗi prompt có CONSTRAINT block** — rules phải tuân thủ
3. **Mỗi prompt có STEPS block** — ordered, cụ thể, không mơ hồ
4. **Mỗi prompt có VERIFY block** — lệnh chạy để verify, expected output
5. **Mỗi prompt có ROLLBACK block** — nếu fail thì làm gì
6. **Không yêu cầu viết code mới** — chỉ MOVE + RENAME + UPDATE IMPORTS + ADD INTERFACES
7. **Ngôn ngữ prompt: English** — Claude Code xử lý English tốt hơn cho code tasks

---

## PROMPT 1 — Phase 0: Audit & Baseline

```
You are helping me refactor the HubLive Server codebase (Go). This is Phase 0 — 
preparation and auditing. NO production code changes in this phase.

## CONTEXT

I'm refactoring hublive-server to split two monolith packages:
- pkg/service/ (31 files — HTTP, RPC, storage, auth mixed together)  
- pkg/rtc/ (29 files — Room, Participant, Track god objects)

Into domain-oriented modules:
- pkg/domain/ (room, participant, track, agent)
- pkg/transport/ (websocket, whip, webrtc)
- pkg/api/ (thin Twirp handlers)
- pkg/store/ (storage abstraction)

This Phase 0 produces AUDIT REPORTS ONLY — zero production code changes.

## CONSTRAINTS

- Do NOT modify any existing .go files
- Do NOT change any tests
- Only CREATE new files: scripts, reports, benchmark files
- Work from the repository root directory
- If a command fails, report the error — do not try to fix production code

## STEPS

### Step 1: Codebase Structure Snapshot

Run these commands and save output to `audit/phase0/structure.txt`:

```bash
mkdir -p audit/phase0

# Package file counts
echo "=== Package File Counts ===" > audit/phase0/structure.txt
find pkg/ -name "*.go" -not -name "*_test.go" | sed 's|/[^/]*$||' | sort | uniq -c | sort -rn >> audit/phase0/structure.txt

echo "" >> audit/phase0/structure.txt

# God object line counts  
echo "=== God Object Line Counts ===" >> audit/phase0/structure.txt
wc -l pkg/rtc/participant.go pkg/rtc/room.go pkg/rtc/transport.go pkg/service/roommanager.go 2>/dev/null >> audit/phase0/structure.txt

echo "" >> audit/phase0/structure.txt

# All files in service/ and rtc/ top-level
echo "=== pkg/service/ files ===" >> audit/phase0/structure.txt
ls -la pkg/service/*.go 2>/dev/null >> audit/phase0/structure.txt
echo "" >> audit/phase0/structure.txt
echo "=== pkg/rtc/ top-level files ===" >> audit/phase0/structure.txt  
ls -la pkg/rtc/*.go 2>/dev/null >> audit/phase0/structure.txt
```

### Step 2: External Consumer Audit

Check if any related HubLive repos import our internal packages.
Since we can't clone external repos, scan go.mod and check if our 
packages are exported in ways that suggest external usage:

```bash
echo "=== External Import Risk ===" > audit/phase0/external-imports.txt

# Check if any package has exported types that match common external usage patterns
echo "--- Exported types in pkg/service/ ---" >> audit/phase0/external-imports.txt
grep -rn "^type [A-Z]" pkg/service/*.go | grep -v "_test.go" >> audit/phase0/external-imports.txt

echo "" >> audit/phase0/external-imports.txt
echo "--- Exported types in pkg/rtc/ (top-level) ---" >> audit/phase0/external-imports.txt
grep -rn "^type [A-Z]" pkg/rtc/*.go | grep -v "_test.go" >> audit/phase0/external-imports.txt

echo "" >> audit/phase0/external-imports.txt
echo "--- Exported types in pkg/rtc/types/ ---" >> audit/phase0/external-imports.txt
grep -rn "^type [A-Z]" pkg/rtc/types/*.go | grep -v "_test.go" >> audit/phase0/external-imports.txt

echo "" >> audit/phase0/external-imports.txt
echo "--- go.mod module path ---" >> audit/phase0/external-imports.txt
head -1 go.mod >> audit/phase0/external-imports.txt
```

### Step 3: Sentinel Error Audit

Find all sentinel errors and custom error types that will move between packages:

```bash
echo "=== Sentinel Errors ===" > audit/phase0/errors.txt

echo "--- pkg/service/ sentinel errors ---" >> audit/phase0/errors.txt
grep -rn 'var Err.*=' pkg/service/*.go | grep -v "_test.go" >> audit/phase0/errors.txt
grep -rn 'errors\.New\|fmt\.Errorf' pkg/service/*.go | grep -v "_test.go" | head -30 >> audit/phase0/errors.txt

echo "" >> audit/phase0/errors.txt
echo "--- pkg/rtc/ sentinel errors ---" >> audit/phase0/errors.txt
grep -rn 'var Err.*=' pkg/rtc/*.go | grep -v "_test.go" >> audit/phase0/errors.txt

echo "" >> audit/phase0/errors.txt
echo "--- Custom error types ---" >> audit/phase0/errors.txt
grep -rn 'type.*Error struct' pkg/service/ pkg/rtc/ pkg/agent/ | grep -v "_test.go" >> audit/phase0/errors.txt

echo "" >> audit/phase0/errors.txt
echo "--- Cross-package error references ---" >> audit/phase0/errors.txt
grep -rn 'service\.Err\|rtc\.Err\|agent\.Err' pkg/ | grep -v "_test.go" | grep -v vendor/ >> audit/phase0/errors.txt
```

### Step 4: servicefakes & typesfakes Usage Audit

Find all test files that import generated fakes — these must be updated per phase:

```bash
echo "=== Fakes Usage ===" > audit/phase0/fakes.txt

echo "--- servicefakes imports ---" >> audit/phase0/fakes.txt
grep -rn 'servicefakes' pkg/ --include="*.go" >> audit/phase0/fakes.txt

echo "" >> audit/phase0/fakes.txt
echo "--- typesfakes imports ---" >> audit/phase0/fakes.txt
grep -rn 'typesfakes' pkg/ --include="*.go" >> audit/phase0/fakes.txt

echo "" >> audit/phase0/fakes.txt
echo "--- counterfeiter generate directives ---" >> audit/phase0/fakes.txt
grep -rn 'go:generate.*counterfeiter' pkg/ --include="*.go" >> audit/phase0/fakes.txt
```

### Step 5: init() Function Audit

Check for init() functions that may have order-dependent side effects:

```bash
echo "=== init() Functions ===" > audit/phase0/init-functions.txt
grep -rn "func init()" pkg/ --include="*.go" | grep -v "_test.go" | grep -v vendor/ | sort >> audit/phase0/init-functions.txt
```

### Step 6: Internal Dependency Map for service/ files being moved

Map which service/ files import which other service/ files:

```bash
echo "=== service/ Internal Dependencies ===" > audit/phase0/service-deps.txt

for f in pkg/service/*.go; do
    [ -f "$f" ] || continue
    echo "--- $(basename $f) imports from service/ ---" >> audit/phase0/service-deps.txt
    # Find imports that reference other files in the same package via type/function usage
    grep -n 'RoomManager\|RoomAllocator\|RTCService\|SignalServer\|ObjectStore\|LocalStore\|RedisStore\|IOService' "$f" | head -10 >> audit/phase0/service-deps.txt
    echo "" >> audit/phase0/service-deps.txt
done
```

### Step 7: Create check-imports.sh Script

Create the import validation script that will be used after every phase:

```bash
cat > scripts/check-imports.sh << 'SCRIPT_EOF'
#!/bin/bash
set -euo pipefail

echo "=== Checking for circular imports ==="
go build ./... 2>&1 | grep -i "import cycle" && {
    echo "FAIL: Circular import detected"
    exit 1
} || echo "PASS: No circular imports"

echo ""
echo "=== Checking dependency rule violations ==="

VIOLATIONS=""

# Domain layer must NOT import infrastructure
if [ -d "pkg/domain" ]; then
    V=$(grep -rn '"github.com/HubLive/hublive-server/pkg/store' pkg/domain/ 2>/dev/null || true)
    VIOLATIONS+="$V"
    V=$(grep -rn '"github.com/HubLive/hublive-server/pkg/transport' pkg/domain/ 2>/dev/null || true)
    VIOLATIONS+="$V"
    V=$(grep -rn '"github.com/HubLive/hublive-server/pkg/routing' pkg/domain/ 2>/dev/null || true)
    VIOLATIONS+="$V"
    V=$(grep -rn '"github.com/HubLive/hublive-server/pkg/api' pkg/domain/ 2>/dev/null || true)
    VIOLATIONS+="$V"
    V=$(grep -rn '"github.com/HubLive/hublive-server/pkg/service' pkg/domain/ 2>/dev/null || true)
    VIOLATIONS+="$V"
fi

if [ -n "$VIOLATIONS" ]; then
    echo "FAIL: Domain layer importing infrastructure:"
    echo "$VIOLATIONS"
    exit 1
fi
echo "PASS: Domain layer imports clean"

echo ""
echo "=== Layer dependency matrix ==="
for dir in domain store transport api; do
    if [ -d "pkg/$dir" ]; then
        echo "$dir/ imports:"
        grep -roh '"github.com/HubLive/hublive-server/pkg/[^"]*"' "pkg/$dir/" 2>/dev/null | sort -u | sed 's/.*pkg\//  /' || echo "  (none)"
        echo ""
    fi
done

echo "=== All checks passed ==="
SCRIPT_EOF

chmod +x scripts/check-imports.sh
```

### Step 8: Verify Current Build & Tests

Before any refactoring, confirm the codebase compiles and tests pass:

```bash
echo "=== Build Check ===" > audit/phase0/build-status.txt
go build ./... >> audit/phase0/build-status.txt 2>&1 && echo "BUILD: PASS" >> audit/phase0/build-status.txt || echo "BUILD: FAIL" >> audit/phase0/build-status.txt

echo "" >> audit/phase0/build-status.txt
echo "=== Vet Check ===" >> audit/phase0/build-status.txt
go vet ./... >> audit/phase0/build-status.txt 2>&1 && echo "VET: PASS" >> audit/phase0/build-status.txt || echo "VET: FAIL" >> audit/phase0/build-status.txt

echo "" >> audit/phase0/build-status.txt
echo "=== Test Check (short mode) ===" >> audit/phase0/build-status.txt
go test -short -count=1 ./pkg/... >> audit/phase0/build-status.txt 2>&1 && echo "TEST: PASS" >> audit/phase0/build-status.txt || echo "TEST: FAIL (see above)" >> audit/phase0/build-status.txt
```

### Step 9: Generate Summary Report

Create a consolidated audit report from all the data collected:

```bash
cat > audit/phase0/SUMMARY.md << 'EOF'
# Phase 0 Audit Summary
Generated: $(date)

## Files to review:
- structure.txt — package sizes and god object line counts
- external-imports.txt — exported types that external repos might use
- errors.txt — sentinel errors that must move with their files
- fakes.txt — counterfeiter fakes that must regenerate per phase
- init-functions.txt — init() functions to audit for order independence
- service-deps.txt — internal coupling within service/ package
- build-status.txt — current build/test status

## Pre-refactor checklist:
- [ ] Review structure.txt — confirm god object sizes match plan
- [ ] Review external-imports.txt — decide if aliases needed long-term
- [ ] Review errors.txt — plan error migration per phase
- [ ] Review fakes.txt — plan fake regeneration per phase
- [ ] Review init-functions.txt — confirm all are order-independent
- [ ] Review service-deps.txt — confirm Phase 2A/2B/3 ordering is correct
- [ ] build-status.txt shows BUILD: PASS and TEST: PASS
EOF
```

After generating the summary, read each audit file and provide me a 
consolidated analysis highlighting:
1. Any SURPRISES vs what the plan expected
2. Any RISKS not covered in the plan
3. CONFIRMATION that Phase 1 can proceed safely

## VERIFY

After all steps:
- [ ] `ls audit/phase0/` shows: structure.txt, external-imports.txt, errors.txt, 
      fakes.txt, init-functions.txt, service-deps.txt, build-status.txt, SUMMARY.md
- [ ] `scripts/check-imports.sh` exists and is executable
- [ ] `cat audit/phase0/build-status.txt` shows BUILD: PASS
- [ ] Zero .go files modified (verify with `git status` or `git diff --stat`)

## ROLLBACK

This phase creates only new files. Rollback = delete:
```bash
rm -rf audit/phase0/
rm -f scripts/check-imports.sh
```
```

---

## PROMPT 1 — Tại sao thiết kế như vậy

### Triết lý

**Audit trước, code sau.** Phase 0 không sửa gì cả — chỉ thu thập data. Nếu audit phát hiện điều bất ngờ (ví dụ: external repo import `pkg/service` types, hoặc có sentinel error được check bằng type assertion cross-package), ta điều chỉnh plan TRƯỚC KHI bắt đầu move code.

### Tại sao 9 steps mà không gộp?

Mỗi step là 1 concern riêng biệt:
- Step 1-2: **Understand** codebase structure + external risk
- Step 3-5: **Inventory** things that will break during move (errors, fakes, init)
- Step 6: **Map** internal coupling (validates Phase 2A/2B/3 order)
- Step 7: **Tooling** cho tất cả phases sau
- Step 8: **Baseline** build/test status
- Step 9: **Consolidate** cho human review

### Tại sao kết thúc bằng "provide analysis"?

Claude Code sẽ đọc audit files và phân tích — đây là giá trị chính. Raw data vô nghĩa nếu không có analysis. Claude Code có thể phát hiện risks mà plan chưa cover, ví dụ:
- "Found 12 servicefakes imports in routing/ tests — Phase 1 will break these"
- "rtc/participant.go has 2 init() functions — need to verify order independence"
- "service/roommanager.go references 15 other service/ files — confirm coupling analysis"

### Prompt 2 preview (Phase 1) sẽ làm gì

Prompt 2 sẽ dựa trên OUTPUT của Prompt 1 để:
1. Tạo `pkg/store/interfaces.go` — copy interface definitions từ `service/interfaces.go`
2. Move `service/localstore.go` → `store/local/store.go`
3. Move `service/redisstore.go` + `redisstore_sip.go` → `store/redis/`
4. Move test files theo
5. Add transition aliases trong `service/compat.go`
6. Regenerate fakes
7. Update ALL imports across codebase
8. Run `go build`, `go test`, `scripts/check-imports.sh`

Mỗi sub-step sẽ có verification riêng.
