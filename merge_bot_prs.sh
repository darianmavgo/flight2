#!/bin/bash

# Repositories to check (from darianmavgo organization)
REPOS=("banquet" "mksqlite" "sqliter" "TableTypeMaster")
ORG="darianmavgo"
BOT_AUTHOR="google-labs-jules"

echo "🤖 Starting to merge pull requests from $BOT_AUTHOR..."

for repo in "${REPOS[@]}"; do
    echo "----------------------------"
    echo "📦 Checking $ORG/$repo..."
    
    # List PRs from the bot
    # We use --json number to get only the PR numbers and --jq to parse it
    PR_NUMBERS=$(gh pr list -R "$ORG/$repo" --author "$BOT_AUTHOR" --json number --jq '.[].number')
    
    if [ -z "$PR_NUMBERS" ]; then
        echo "✅ No open pull requests found from $BOT_AUTHOR in $repo."
        continue
    fi
    
    for pr in $PR_NUMBERS; do
        echo "🚀 Merging PR #$pr in $repo..."
        # Using --merge for a standard merge commit. 
        # Alternatively, use --squash if preferred.
        if gh pr merge "$pr" -R "$ORG/$repo" --merge --admin; then
            echo "🎉 Successfully merged PR #$pr in $repo"
        else
            echo "❌ Failed to merge PR #$pr in $repo. You might need to check if checks are passing or if you have permissions."
        fi
    done
done

echo "----------------------------"
echo "🏁 Bot PR merge process complete."
