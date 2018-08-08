#!/bin/bash

kubectl delete -f sample/rss/flow.yaml
sleep 2
kubectl delete feeds/rss-flow
sleep 2
ko delete -f pkg/sources/rss/
