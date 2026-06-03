#!/bin/bash

echo "=========================================="
echo "CloudFlow Repository Status Check"
echo "=========================================="
echo ""

cd /opt/cloudflow

echo "1. Current Branch:"
git branch --show-current
echo ""

echo "2. Last 3 Commits:"
git log --pretty=format:"%h - %s (%an, %ar)" -3
echo ""
echo ""

echo "3. Uncommitted Changes:"
git status -s
echo ""

echo "4. Tags:"
git tag -l | tail -5
echo ""

echo "5. Remote URL:"
git remote get-url origin
echo ""

echo "=========================================="
echo "Check Complete!"
echo "=========================================="
