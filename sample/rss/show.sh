#!/bin/bash


kubectl get buses stub -oyaml
echo '---'
kubectl get channels rss-flow -oyaml
echo '---'
kubectl get eventsources rss -oyaml
echo '---'
kubectl get eventtypes dev.knative.rss.event -oyaml
echo '---'
kubectl get feeds rss-flow -oyaml
echo '---'
kubectl get flows rss-flow -oyaml
echo '---'
kubectl get subscriptions rss-flow -oyaml
