output "cluster_name" {
  value = google_container_cluster.this.name
}

output "cluster_endpoint" {
  value     = google_container_cluster.this.endpoint
  sensitive = true
}

output "cluster_id" {
  value = google_container_cluster.this.id
}

output "workload_pool" {
  value = "${var.project_id}.svc.id.goog"
}
