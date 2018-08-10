#!/bin/bash

ko apply -f pkg/sources/rss/

kubectl apply -f sample/rss/auth.yaml
kubectl apply -f sample/rss/serviceentry.yaml
kubectl apply -f sample/rss/flow.yaml
