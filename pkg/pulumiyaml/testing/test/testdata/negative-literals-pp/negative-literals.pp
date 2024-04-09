resource ecsTarget1 "aws:appautoscaling:Target" {
  maxCapacity = 4
  minCapacity = 1
  resourceId = "service/clustername/serviceName"
  scalableDimension = "ecs:service:DesiredCount"
  serviceNamespace = "ecs"
}

resource ecsPolicy1 "aws:appautoscaling:Policy" {
  name = "scale-down-integers"
  policyType = "StepScaling"
  resourceId = ecsTarget1.resourceId
  scalableDimension = ecsTarget1.scalableDimension
  serviceNamespace = ecsTarget1.serviceNamespace

  stepScalingPolicyConfiguration = {
    adjustmentType = "ChangeInCapacity"
    cooldown = 60
    metricAggregationType = "Maximum"

    stepAdjustments = [
      {
        metricIntervalUpperBound = 0
        scalingAdjustment = -1
      }
    ]
  }
}

resource ecsTarget2 "aws:appautoscaling:Target" {
  maxCapacity = 4
  minCapacity = 1
  resourceId = "service/clustername/serviceName"
  scalableDimension = "ecs:service:DesiredCount"
  serviceNamespace = "ecs"
}

resource ecsPolicy2 "aws:appautoscaling:Policy" {
  name = "scale-down-floats"
  policyType = "StepScaling"
  resourceId = ecsTarget2.resourceId
  scalableDimension = ecsTarget2.scalableDimension
  serviceNamespace = ecsTarget2.serviceNamespace

  stepScalingPolicyConfiguration = {
    adjustmentType = "ChangeInCapacity"
    cooldown = 60
    metricAggregationType = "Maximum"

    stepAdjustments = [
      {
        metricIntervalUpperBound = 0
        scalingAdjustment = -2.0
      }
    ]
  }
}
