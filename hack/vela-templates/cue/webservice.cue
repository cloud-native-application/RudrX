output: {
  apiVersion: "apps/v1"
  kind:       "Deployment"
  metadata: name: context.name
  spec: {
    replicas: 1

    template: {
      metadata:
        labels:
          "component.oam.dev/name": context.name
          
      spec: {
        containers: [{
          name:  context.name
          image: parameter.image
          if parameter["env"] != _|_ {
            env: parameter.env
          }
          ports: [{
            containerPort: parameter.port
          }]
        }]
      }
    }

    selector: 
      matchLabels:
        "component.oam.dev/name": context.name
  }
}
parameter: {
  // +usage=specify app image
  // +short=i
  image: string

  // +usage=specify port for container
  // +short=p
  port:  *6379 | int

  env?: [...{
    name:  string
    value?: string
    valueFrom?: {
      secretKeyRef: {
        name: string
        key: string
      }
    }
  }]
}
